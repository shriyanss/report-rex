package cmd

import (
	"fmt"

	"github.com/shriyanss/report-rex/cli/internal/section"
	"github.com/spf13/cobra"
)

var reportSectionCmd = &cobra.Command{
	Use:   "section",
	Short: "Add a new vuln, appendix, or narrative section to an existing report",
}

// ── vuln ─────────────────────────────────────────────────────────────────

var (
	vulnOpts          section.VulnOptions
	vulnListTemplates bool
)

var reportSectionVulnCmd = &cobra.Command{
	Use:   "vuln",
	Short: "Add a new finding to reports/ and register it in index.md",
	RunE: func(cmd *cobra.Command, args []string) error {
		if vulnListTemplates {
			names, err := section.VulnTemplateNames()
			if err != nil {
				return err
			}
			for _, name := range names {
				fmt.Printf("%s\t%s\n", name, section.VulnTemplateTitle(name))
			}
			return nil
		}
		relPath, note, err := section.AddVuln(sectionDir, vulnOpts)
		if err != nil {
			return err
		}
		fmt.Printf("Created %s\n", relPath)
		if note != "" {
			fmt.Println(note)
		}
		return nil
	},
}

// ── appendix ─────────────────────────────────────────────────────────────

var appendixTitle string

var reportSectionAppendixCmd = &cobra.Command{
	Use:   "appendix",
	Short: "Add a new appendix and register it in index.md",
	RunE: func(cmd *cobra.Command, args []string) error {
		relPath, note, err := section.AddAppendix(sectionDir, appendixTitle)
		if err != nil {
			return err
		}
		fmt.Printf("Created %s\n", relPath)
		if note != "" {
			fmt.Println(note)
		}
		return nil
	},
}

// ── narrative ────────────────────────────────────────────────────────────

var (
	narrativeName  string
	narrativeTitle string
)

var reportSectionNarrativeCmd = &cobra.Command{
	Use:   "narrative",
	Short: "Add a new narrative section (e.g. exec summary, methodology) and register it in index.md",
	RunE: func(cmd *cobra.Command, args []string) error {
		relPath, note, err := section.AddSection(sectionDir, narrativeName, narrativeTitle)
		if err != nil {
			return err
		}
		fmt.Printf("Created %s\n", relPath)
		if note != "" {
			fmt.Println(note)
		}
		return nil
	},
}

var sectionDir string

func init() {
	reportSectionCmd.PersistentFlags().StringVar(&sectionDir, "dir", ".", "report directory")

	vf := reportSectionVulnCmd.Flags()
	vf.StringVar(&vulnOpts.Title, "title", "", "finding title (required unless --template is given)")
	vf.StringVar(&vulnOpts.Template, "template", "", "start from a vuln-templates/<name>.md stub instead of a blank finding")
	vf.StringVar(&vulnOpts.File, "file", "", "write reports/<file>.md instead of the next reports/vuln-N.md")
	vf.BoolVar(&vulnListTemplates, "list-templates", false, "list the available --template names (name<TAB>title) and exit")
	vf.StringVar(&vulnOpts.CVSSVector, "cvss-vector", "", "CVSS v3.1 vector string")
	vf.StringVar(&vulnOpts.Component, "component", "", "affected component")
	vf.StringVar(&vulnOpts.CWE, "cwe", "", "CWE identifier, e.g. CWE-89")
	vf.StringVar(&vulnOpts.OWASP, "owasp", "", "OWASP Top 10 category, e.g. A03:2021")
	vf.StringVar(&vulnOpts.MITRE, "mitre", "", "MITRE ATT&CK technique, e.g. T1190")
	vf.StringVar(&vulnOpts.Mitigations, "mitigations", "", "MITRE mitigation IDs, e.g. \"M1032, M1038\"")
	vf.StringVar(&vulnOpts.Impact, "impact", "", "impact rating, e.g. Critical|High|Medium|Low")
	vf.StringVar(&vulnOpts.Sophistication, "sophistication", "", "attacker sophistication required")
	vf.StringVar(&vulnOpts.MaterialImpact, "material-impact", "", "material impact category")
	vf.StringArrayVar(&vulnOpts.BusinessRisk, "business-risk", nil, "business risk category (repeatable)")
	vf.StringArrayVar(&vulnOpts.AffectedURLs, "affected-url", nil, "affected URL (repeatable)")
	vf.StringVar(&vulnOpts.TimelineDate, "timeline-date", "", "timeline entry date, RFC3339 (requires --timeline-desc)")
	vf.StringVar(&vulnOpts.TimelineDesc, "timeline-desc", "", "timeline entry description (requires --timeline-date)")

	af := reportSectionAppendixCmd.Flags()
	af.StringVar(&appendixTitle, "title", "", "appendix title")
	_ = reportSectionAppendixCmd.MarkFlagRequired("title")

	nf := reportSectionNarrativeCmd.Flags()
	nf.StringVar(&narrativeName, "name", "", "filename slug, written to sections/<name>.md")
	nf.StringVar(&narrativeTitle, "title", "", "section title")
	_ = reportSectionNarrativeCmd.MarkFlagRequired("name")
	_ = reportSectionNarrativeCmd.MarkFlagRequired("title")

	reportSectionCmd.AddCommand(reportSectionVulnCmd, reportSectionAppendixCmd, reportSectionNarrativeCmd)
	reportCmd.AddCommand(reportSectionCmd)
}
