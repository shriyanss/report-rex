// Package handoff implements the handoffs/<assignee>/<title>.md namespace:
// frontmatter metadata, path/metadata consistency validation, and the small
// set of in-place frontmatter edits (stamping identity on create, marking
// resolved) that both the client and the sync server need. It deliberately
// depends on neither syncclient nor syncserver so both can import it.
package handoff

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/shriyanss/report-rex/cli/internal/report"
)

// Dir is the workspace-relative root of the handoff namespace.
const Dir = "handoffs"

// Handoff lifecycle states. The initial lifecycle is deliberately just
// these two; CANCELLED/REOPENED can be added later without a migration
// since state lives in the file itself.
const (
	StatusOpen     = "open"
	StatusResolved = "resolved"
)

// Frontmatter keys.
const (
	KeyID         = "rex_handoff_id"
	KeyFrom       = "from"
	KeyTo         = "to"
	KeyStatus     = "status"
	KeyCreatedAt  = "created_at"
	KeyResolvedBy = "resolved_by"
	KeyResolvedAt = "resolved_at"
)

// Meta is a handoff's frontmatter metadata.
type Meta struct {
	ID         string
	From       string
	To         string
	Status     string
	CreatedAt  string
	ResolvedBy string
	ResolvedAt string
}

// Handoff is a parsed, validated handoff file.
type Handoff struct {
	Path  string // handoffs/<assignee>/<title>.md
	Title string // <title>, i.e. the filename without its .md extension
	Meta
	Body string
}

// Resolved reports whether this handoff is no longer active.
func (m Meta) Resolved() bool { return m.Status == StatusResolved }

// ValidationError reports a handoff whose directory and `to` metadata
// disagree, or which is missing the `to` metadata entirely. Rex never picks
// a winner between the two -- an ambiguous assignment would risk delivering
// the handoff to the wrong user -- so this is always surfaced, never
// silently repaired.
type ValidationError struct {
	Path         string
	DirAssignee  string
	MetaAssignee string
}

func (e *ValidationError) Error() string {
	if e.MetaAssignee == "" {
		return fmt.Sprintf(
			"invalid handoff metadata\n\n%s\n\nDirectory assignee: %s\nMetadata assignee:  (missing)\n\nThe handoff must declare a `to` assignee matching its directory.",
			e.Path, e.DirAssignee)
	}
	return fmt.Sprintf(
		"invalid handoff metadata\n\n%s\n\nDirectory assignee: %s\nMetadata assignee:  %s\n\nThe handoff directory and `to` metadata must match.",
		e.Path, e.DirAssignee, e.MetaAssignee)
}

// IsPath reports whether a normalized, workspace-relative path names a
// handoff file: exactly handoffs/<assignee>/<title>.md. Anything else under
// handoffs/ (a bare file, a deeper nesting, a non-markdown file) is not a
// handoff, so callers must reject rather than guess at its assignee.
func IsPath(p string) bool {
	segs := strings.Split(p, "/")
	if len(segs) != 3 || segs[0] != Dir {
		return false
	}
	if segs[1] == "" || segs[2] == "" {
		return false
	}
	return strings.HasSuffix(segs[2], ".md") && len(segs[2]) > len(".md")
}

// InNamespace reports whether a path lives anywhere under handoffs/,
// including malformed ones that IsPath rejects.
func InNamespace(p string) bool {
	return p == Dir || strings.HasPrefix(p, Dir+"/")
}

// ScopePrefix returns the path prefix covering one user's handoff queue, or
// the whole namespace when user is empty.
func ScopePrefix(user string) string {
	if user == "" {
		return Dir + "/"
	}
	return Dir + "/" + user + "/"
}

// AssigneeFromPath returns the <assignee> segment of a handoff path.
func AssigneeFromPath(p string) string {
	segs := strings.Split(p, "/")
	if len(segs) != 3 {
		return ""
	}
	return segs[1]
}

// TitleFromPath returns the <title> segment of a handoff path, without the
// .md extension.
func TitleFromPath(p string) string {
	segs := strings.Split(p, "/")
	if len(segs) != 3 {
		return ""
	}
	return strings.TrimSuffix(segs[2], ".md")
}

// PathFor builds the canonical path for a handoff.
func PathFor(assignee, title string) string {
	return Dir + "/" + assignee + "/" + strings.TrimSuffix(title, ".md") + ".md"
}

// Parse reads a handoff's frontmatter and validates that the directory it
// lives in agrees with its `to` metadata. A *ValidationError is returned on
// disagreement; the handoff must not be synchronized until corrected.
func Parse(path string, content []byte) (*Handoff, error) {
	if !IsPath(path) {
		return nil, fmt.Errorf("not a handoff path: %s (expected %s/<assignee>/<title>.md)", path, Dir)
	}
	meta := ParseMeta(content)
	dirAssignee := AssigneeFromPath(path)
	if meta.To != dirAssignee {
		return nil, &ValidationError{Path: path, DirAssignee: dirAssignee, MetaAssignee: meta.To}
	}
	_, body := report.SplitFrontmatter(string(content))
	return &Handoff{Path: path, Title: TitleFromPath(path), Meta: meta, Body: body}, nil
}

// ParseMeta extracts handoff metadata from a file's frontmatter without
// validating it against a path. Missing or malformed frontmatter yields a
// zero Meta rather than an error -- validation is Parse's job.
func ParseMeta(content []byte) Meta {
	fm := report.ParseFrontmatter(string(content))
	return Meta{
		ID:         stringField(fm, KeyID),
		From:       stringField(fm, KeyFrom),
		To:         stringField(fm, KeyTo),
		Status:     stringField(fm, KeyStatus),
		CreatedAt:  stringField(fm, KeyCreatedAt),
		ResolvedBy: stringField(fm, KeyResolvedBy),
		ResolvedAt: stringField(fm, KeyResolvedAt),
	}
}

func stringField(fm map[string]any, key string) string {
	switch v := fm[key].(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		// A YAML timestamp scalar (created_at without quotes) decodes to
		// time.Time, and an all-digit id decodes to int -- render either
		// back to the text form the wire protocol carries.
		if t, ok := v.(time.Time); ok {
			return t.Format(time.RFC3339)
		}
		return fmt.Sprint(v)
	}
}

// Stamp fills in the identity fields a hand-written handoff omits --
// rex_handoff_id, from, created_at, status -- leaving any value the file
// already carries untouched. This is what lets a user create a handoff by
// writing nothing but `to:` and a body. It reports whether anything
// changed, so callers can skip rewriting an already-complete file.
func Stamp(content []byte, from string) ([]byte, bool, error) {
	meta := ParseMeta(content)
	adds := map[string]string{}
	if meta.ID == "" {
		adds[KeyID] = NewID()
	}
	if meta.From == "" {
		adds[KeyFrom] = from
	}
	if meta.CreatedAt == "" {
		adds[KeyCreatedAt] = Now()
	}
	if meta.Status == "" {
		adds[KeyStatus] = StatusOpen
	}
	if len(adds) == 0 {
		return content, false, nil
	}
	out, err := setFrontmatter(content, adds)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

// SetResolved records the terminal state of a handoff in its frontmatter.
// The result is what gets stored as the handoff's final canonical revision.
func SetResolved(content []byte, by, at string) ([]byte, error) {
	return setFrontmatter(content, map[string]string{
		KeyStatus:     StatusResolved,
		KeyResolvedBy: by,
		KeyResolvedAt: at,
	})
}

// setFrontmatter overwrites the given keys in a file's YAML frontmatter,
// preserving the markdown body verbatim.
//
// ponytail: round-trips the frontmatter through map[string]any, so yaml.v3
// re-emits keys in sorted order and drops any comments. Handoff frontmatter
// is machine-managed metadata where that's harmless; switch to yaml.Node if
// handoffs ever need ordered or commented frontmatter.
func setFrontmatter(content []byte, set map[string]string) ([]byte, error) {
	fmText, body := report.SplitFrontmatter(string(content))
	meta := map[string]any{}
	if fmText != "" {
		if err := yaml.Unmarshal([]byte(fmText), &meta); err != nil {
			return nil, fmt.Errorf("parse handoff frontmatter: %w", err)
		}
		if meta == nil {
			meta = map[string]any{}
		}
	}
	for k, v := range set {
		meta[k] = v
	}
	encoded, err := yaml.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("encode handoff frontmatter: %w", err)
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(encoded)
	b.WriteString("---\n")
	if body != "" {
		b.WriteString("\n")
		b.WriteString(body)
		b.WriteString("\n")
	}
	return []byte(b.String()), nil
}

// NewID returns a short, stable handoff identifier. crypto/rand.Read is
// documented never to fail, so there is no error path to handle here.
func NewID() string {
	var b [3]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Now is the timestamp format used for created_at/resolved_at.
func Now() string { return time.Now().Format(time.RFC3339) }
