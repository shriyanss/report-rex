package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/shriyanss/report-rex/cli/internal/report"
	"github.com/shriyanss/report-rex/cli/internal/section"
	"github.com/shriyanss/report-rex/cli/internal/syncclient"
	"github.com/shriyanss/report-rex/cli/internal/version"
)

// mcpFlags mirrors syncFlags but drops its --dir default clash: this
// command's --dir names the report workspace, not a sync workspace, and
// sync is optional here (an empty --host just means "write locally only").
type mcpFlags struct {
	Dir      string
	Host     string
	Port     int
	Username string
	Password string
	CACert   string
	Insecure bool
}

var mcpFlagsVal mcpFlags

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run an MCP server so an AI harness can add/update findings without touching report files directly",
	RunE:  runMCP,
}

func init() {
	f := mcpCmd.Flags()
	f.StringVar(&mcpFlagsVal.Dir, "dir", ".", "report workspace directory")
	f.StringVar(&mcpFlagsVal.Host, "host", "", "rex-svr host to sync AI-driven changes through (optional; omit to write locally only)")
	f.IntVar(&mcpFlagsVal.Port, "port", 9443, "rex-svr port")
	f.StringVar(&mcpFlagsVal.Username, "username", "mcp", "username to authenticate to rex-svr as (each MCP-connected harness should use its own)")
	f.StringVar(&mcpFlagsVal.Password, "password", "", "shared server password (falls back to REX_PASSWORD)")
	f.StringVar(&mcpFlagsVal.CACert, "ca-cert", "", "path to rex-svr's self-signed certificate, to trust it")
	f.BoolVar(&mcpFlagsVal.Insecure, "insecure", false, "skip TLS certificate verification (use only on a trusted network, as an alternative to --ca-cert)")

	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, args []string) error {
	flags := mcpFlagsVal
	dirAbs, err := filepath.Abs(flags.Dir)
	if err != nil {
		return fmt.Errorf("resolving --dir: %w", err)
	}

	srv := &mcpServer{dir: dirAbs, username: flags.Username}

	if flags.Host != "" {
		// A stdio MCP server's stdin is the JSON-RPC channel to the
		// harness, not a TTY, so unlike `rex sync` this can't fall back to
		// an interactive password prompt.
		password := flags.Password
		if password == "" {
			password = os.Getenv("REX_PASSWORD")
		}
		if password == "" {
			return fmt.Errorf("--password or REX_PASSWORD is required when --host is set (rex mcp cannot prompt interactively over stdio)")
		}
		tlsConfig, err := syncclient.BuildTLSConfig(flags.CACert, flags.Insecure)
		if err != nil {
			return err
		}
		client := syncclient.NewClient(flags.Host, flags.Port, tlsConfig)
		if _, err := client.Login(cmd.Context(), flags.Username, password); err != nil {
			return fmt.Errorf("login to rex-svr: %w", err)
		}
		local, err := syncclient.OpenLocalStore(dirAbs)
		if err != nil {
			return fmt.Errorf("opening local sync store: %w", err)
		}
		defer local.Close()
		srv.client = client
		srv.engine = &syncclient.Engine{Dir: dirAbs, Client: client, Local: local, Username: flags.Username}
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "report-rex", Version: version.Version}, nil)
	srv.registerTools(server)
	return server.Run(cmd.Context(), &mcp.StdioTransport{})
}

// mcpServer holds the shared state every MCP tool handler needs: the report
// workspace, and, if --host was given, a logged-in sync client/engine for
// pushing AI-driven changes to rex-svr under their own identity.
type mcpServer struct {
	dir      string
	username string
	client   *syncclient.Client
	engine   *syncclient.Engine
}

func (s *mcpServer) registerTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_findings",
		Description: "List every vulnerability finding in the report workspace: path, title, severity, status, and whether it's AI-owned.",
	}, s.listFindings)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_finding",
		Description: "Read one finding's full frontmatter and markdown body.",
	}, s.getFinding)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_finding",
		Description: "Create a new vulnerability finding (reports/vuln-N.md) and register it in index.md. Always stamped ai: true. If a body is given, it must include a one-liner curl PoC command.",
	}, s.addFinding)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_finding",
		Description: "Update an existing finding's fields and/or body. Refuses findings that aren't already ai: true unless user_authorized is set — only set that when the human user explicitly told you to change a finding you didn't create. If a body is given, it must include a one-liner curl PoC command.",
	}, s.updateFinding)
}

// resolveFindingPath confirms relPath names a file under reports/ inside
// this server's workspace, rejecting anything that would escape it — the
// harness supplies this path, so it's a trust boundary.
func (s *mcpServer) resolveFindingPath(relPath string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("path is required")
	}
	clean := filepath.Clean(relPath)
	if clean != "reports" && !strings.HasPrefix(clean, "reports"+string(filepath.Separator)) {
		return "", fmt.Errorf("path must be under reports/, got %q", relPath)
	}
	abs := filepath.Join(s.dir, clean)
	if !strings.HasPrefix(abs, s.dir+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the report workspace: %q", relPath)
	}
	return abs, nil
}

// pushIfSynced pushes relPath (via a full reconcile pass, the same
// primitive `rex sync` uses) to rex-svr, if sync is configured. For an
// update to an existing file, it first checks the presence snapshot: if
// another session currently has relPath open, it waits 5 seconds before
// pushing rather than racing that edit.
func (s *mcpServer) pushIfSynced(ctx context.Context, relPath string, checkPresence bool) error {
	if s.engine == nil {
		return nil
	}
	if checkPresence {
		if leases, err := s.client.Presence(ctx); err == nil {
			for _, l := range leases {
				if l.Path == relPath && l.Username != s.username {
					time.Sleep(5 * time.Second)
					break
				}
			}
		}
	}
	_, err := s.engine.Reconcile(ctx)
	return err
}

type vulnFields struct {
	Title          string   `json:"title,omitempty" jsonschema:"finding title"`
	CVSSVector     string   `json:"cvss_vector,omitempty" jsonschema:"e.g. CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H; drives auto severity+score"`
	Component      string   `json:"component,omitempty"`
	CWE            string   `json:"cwe,omitempty" jsonschema:"e.g. CWE-89"`
	OWASP          string   `json:"owasp,omitempty" jsonschema:"e.g. A03:2021"`
	MITRE          string   `json:"mitre,omitempty" jsonschema:"MITRE ATT&CK technique, e.g. T1190"`
	Mitigations    string   `json:"mitigations,omitempty" jsonschema:"MITRE mitigation codes, comma-separated"`
	Impact         string   `json:"impact,omitempty"`
	Sophistication string   `json:"sophistication,omitempty"`
	MaterialImpact string   `json:"material_impact,omitempty" jsonschema:"FAIR-MAM category"`
	BusinessRisk   []string `json:"business_risk,omitempty" jsonschema:"Strategic|Operational|Financial|Compliance|Reputational"`
	AffectedURLs   []string `json:"affected_urls,omitempty"`
	TimelineDate   string   `json:"timeline_date,omitempty" jsonschema:"ISO 8601 date for a timeline entry"`
	TimelineDesc   string   `json:"timeline_description,omitempty"`
	Body           string   `json:"body,omitempty" jsonschema:"full markdown body: ## Description / ## Impact / ## Replication (steps and evidence) / ## Remediation. Replication MUST include a one-liner curl PoC command so a human triager can reproduce it immediately."`
}

func (f vulnFields) toVulnOptions() section.VulnOptions {
	return section.VulnOptions{
		Title: f.Title, CVSSVector: f.CVSSVector, Component: f.Component, CWE: f.CWE,
		OWASP: f.OWASP, MITRE: f.MITRE, Mitigations: f.Mitigations, Impact: f.Impact,
		Sophistication: f.Sophistication, MaterialImpact: f.MaterialImpact,
		BusinessRisk: f.BusinessRisk, AffectedURLs: f.AffectedURLs,
		TimelineDate: f.TimelineDate, TimelineDesc: f.TimelineDesc, Body: f.Body,
	}
}

type listFindingsInput struct{}

type findingSummary struct {
	Path     string `json:"path"`
	Title    string `json:"title,omitempty"`
	Severity string `json:"severity,omitempty"`
	Status   string `json:"status,omitempty"`
	AI       bool   `json:"ai"`
}

type listFindingsOutput struct {
	Findings []findingSummary `json:"findings"`
}

func (s *mcpServer) listFindings(_ context.Context, _ *mcp.CallToolRequest, _ listFindingsInput) (*mcp.CallToolResult, listFindingsOutput, error) {
	structure, err := report.ParseIndex(filepath.Join(s.dir, "index.md"))
	if err != nil {
		return nil, listFindingsOutput{}, err
	}
	var out listFindingsOutput
	for _, abs := range structure.Reports {
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil, listFindingsOutput{}, fmt.Errorf("reading %s: %w", abs, err)
		}
		meta := report.ParseFrontmatter(string(data))
		rel, _ := filepath.Rel(s.dir, abs)
		fs := findingSummary{Path: filepath.ToSlash(rel)}
		fs.Title, _ = meta["title"].(string)
		fs.Severity, _ = meta["severity"].(string)
		fs.Status, _ = meta["status"].(string)
		fs.AI, _ = meta["ai"].(bool)
		out.Findings = append(out.Findings, fs)
	}
	return nil, out, nil
}

type findingPathInput struct {
	Path string `json:"path" jsonschema:"finding path relative to the report workspace, e.g. reports/vuln-2.md"`
}

type getFindingOutput struct {
	Frontmatter map[string]any `json:"frontmatter"`
	Body        string         `json:"body"`
}

func (s *mcpServer) getFinding(_ context.Context, _ *mcp.CallToolRequest, in findingPathInput) (*mcp.CallToolResult, getFindingOutput, error) {
	abs, err := s.resolveFindingPath(in.Path)
	if err != nil {
		return nil, getFindingOutput{}, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, getFindingOutput{}, fmt.Errorf("reading %s: %w", in.Path, err)
	}
	_, body := report.SplitFrontmatter(string(data))
	return nil, getFindingOutput{Frontmatter: report.ParseFrontmatter(string(data)), Body: body}, nil
}

type addFindingInput struct {
	vulnFields
	Template string `json:"template,omitempty" jsonschema:"optional vuln-templates/<name>.md starter to base the finding on; title is required unless this is set"`
}

type findingResult struct {
	Path string `json:"path"`
	Note string `json:"note,omitempty"`
}

func (s *mcpServer) addFinding(ctx context.Context, _ *mcp.CallToolRequest, in addFindingInput) (*mcp.CallToolResult, findingResult, error) {
	opts := in.toVulnOptions()
	opts.Template = in.Template
	opts.AI = true // the MCP path is the only caller allowed to stamp ai: true on creation

	relPath, note, err := section.AddVuln(s.dir, opts)
	if err != nil {
		return nil, findingResult{}, err
	}
	if err := s.pushIfSynced(ctx, relPath, false); err != nil {
		return nil, findingResult{}, fmt.Errorf("created %s locally but failed to sync: %w", relPath, err)
	}
	return nil, findingResult{Path: relPath, Note: note}, nil
}

type updateFindingInput struct {
	vulnFields
	Path           string `json:"path" jsonschema:"finding path to update, e.g. reports/vuln-2.md"`
	UserAuthorized bool   `json:"user_authorized,omitempty" jsonschema:"set true only when the human user explicitly told you to modify a finding you didn't create"`
}

func (s *mcpServer) updateFinding(ctx context.Context, _ *mcp.CallToolRequest, in updateFindingInput) (*mcp.CallToolResult, findingResult, error) {
	if _, err := s.resolveFindingPath(in.Path); err != nil {
		return nil, findingResult{}, err
	}
	note, err := section.UpdateVuln(s.dir, in.Path, in.toVulnOptions(), in.UserAuthorized)
	if err != nil {
		return nil, findingResult{}, err
	}
	if err := s.pushIfSynced(ctx, in.Path, true); err != nil {
		return nil, findingResult{}, fmt.Errorf("updated %s locally but failed to sync: %w", in.Path, err)
	}
	return nil, findingResult{Path: in.Path, Note: note}, nil
}
