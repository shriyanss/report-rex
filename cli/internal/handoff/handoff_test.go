package handoff

import (
	"strings"
	"testing"
)

func TestIsPath(t *testing.T) {
	cases := map[string]bool{
		"handoffs/shriyanss/nextjs.md":      true,
		"handoffs/alice/investigate-api.md": true,
		"handoffs/nextjs.md":                false, // no assignee directory
		"handoffs/alice/sub/nextjs.md":      false, // too deep to name an assignee
		"handoffs/alice/notes.txt":          false, // not markdown
		"handoffs/alice/.md":                false, // empty title
		"handoffs":                          false,
		"reports/vuln-1.md":                 false,
		"sections/handoffs/alice/nextjs.md": false,
	}
	for p, want := range cases {
		if got := IsPath(p); got != want {
			t.Errorf("IsPath(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestInNamespaceAndScopePrefix(t *testing.T) {
	if !InNamespace("handoffs/alice/x.md") || !InNamespace("handoffs") {
		t.Error("InNamespace should match the handoffs namespace")
	}
	if InNamespace("handoffsomething/x.md") {
		t.Error("InNamespace must not match a directory that merely shares the prefix")
	}
	if got := ScopePrefix("alice"); got != "handoffs/alice/" {
		t.Errorf("ScopePrefix(alice) = %q", got)
	}
	if got := ScopePrefix(""); got != "handoffs/" {
		t.Errorf("ScopePrefix(all) = %q", got)
	}
}

const validHandoff = `---
rex_handoff_id: 8f83c1
from: alice
to: shriyanss
status: open
created_at: "2026-08-22T17:15:00-04:00"
---

# Investigate Next.js authentication

The application appears to use Next.js.
`

func TestParseValid(t *testing.T) {
	h, err := Parse("handoffs/shriyanss/investigate-nextjs-auth.md", []byte(validHandoff))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if h.ID != "8f83c1" || h.From != "alice" || h.To != "shriyanss" || h.Status != StatusOpen {
		t.Errorf("unexpected metadata: %+v", h.Meta)
	}
	if h.Title != "investigate-nextjs-auth" {
		t.Errorf("Title = %q", h.Title)
	}
	if !strings.Contains(h.Body, "appears to use Next.js") {
		t.Errorf("Body not extracted: %q", h.Body)
	}
	if h.Resolved() {
		t.Error("open handoff reported as resolved")
	}
}

func TestParseMismatchIsRejected(t *testing.T) {
	// handoffs/alice/nextjs.md declaring `to: shriyanss` is ambiguous: Rex
	// must not pick either value.
	_, err := Parse("handoffs/alice/nextjs.md", []byte(validHandoff))
	if err == nil {
		t.Fatal("expected a validation error for a dir/metadata mismatch")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if ve.DirAssignee != "alice" || ve.MetaAssignee != "shriyanss" {
		t.Errorf("unexpected error fields: %+v", ve)
	}
	msg := ve.Error()
	for _, want := range []string{
		"invalid handoff metadata",
		"handoffs/alice/nextjs.md",
		"Directory assignee: alice",
		"Metadata assignee:  shriyanss",
		"The handoff directory and `to` metadata must match.",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q:\n%s", want, msg)
		}
	}
}

func TestParseMissingToIsRejected(t *testing.T) {
	_, err := Parse("handoffs/alice/x.md", []byte("# no frontmatter at all\n"))
	if err == nil {
		t.Fatal("expected a validation error for missing `to` metadata")
	}
	if !strings.Contains(err.Error(), "(missing)") {
		t.Errorf("expected a missing-assignee message, got:\n%s", err)
	}
}

func TestParseRejectsNonHandoffPath(t *testing.T) {
	if _, err := Parse("reports/vuln-1.md", []byte(validHandoff)); err == nil {
		t.Fatal("expected an error for a non-handoff path")
	}
}

func TestStampFillsOnlyMissingFields(t *testing.T) {
	// The issue's create flow: a user writes nothing but `to:` and a body.
	src := []byte("---\nto: shriyanss\n---\n\n# Investigate Next.js auth\n\nPlease look at this.\n")
	out, changed, err := Stamp(src, "alice")
	if err != nil {
		t.Fatalf("Stamp: %v", err)
	}
	if !changed {
		t.Fatal("Stamp reported no change on an unstamped handoff")
	}
	h, err := Parse("handoffs/shriyanss/investigate-nextjs-auth.md", out)
	if err != nil {
		t.Fatalf("Parse after Stamp: %v", err)
	}
	if len(h.ID) != 6 {
		t.Errorf("expected a 6-char id, got %q", h.ID)
	}
	if h.From != "alice" || h.Status != StatusOpen || h.CreatedAt == "" {
		t.Errorf("Stamp did not fill identity fields: %+v", h.Meta)
	}
	if !strings.Contains(h.Body, "Please look at this.") {
		t.Errorf("Stamp lost the body: %q", h.Body)
	}

	// Re-stamping is a no-op: an existing `from` is never overwritten, so a
	// later sync by a different user can't steal authorship.
	again, changed, err := Stamp(out, "bob")
	if err != nil {
		t.Fatalf("re-Stamp: %v", err)
	}
	if changed {
		t.Error("re-stamping an already-complete handoff should report no change")
	}
	if h2 := ParseMeta(again); h2.From != "alice" || h2.ID != h.ID {
		t.Errorf("re-stamping mutated identity: %+v", h2)
	}
}

func TestSetResolved(t *testing.T) {
	out, err := SetResolved([]byte(validHandoff), "shriyanss", "2026-08-22T18:03:17-04:00")
	if err != nil {
		t.Fatalf("SetResolved: %v", err)
	}
	h, err := Parse("handoffs/shriyanss/nextjs.md", out)
	if err != nil {
		t.Fatalf("Parse after SetResolved: %v", err)
	}
	if !h.Resolved() {
		t.Errorf("status = %q, want resolved", h.Status)
	}
	if h.ResolvedBy != "shriyanss" || h.ResolvedAt != "2026-08-22T18:03:17-04:00" {
		t.Errorf("unexpected resolution metadata: %+v", h.Meta)
	}
	// Identity and body survive resolution -- the canonical file stays
	// readable as history.
	if h.ID != "8f83c1" || h.From != "alice" {
		t.Errorf("resolution mutated identity: %+v", h.Meta)
	}
	if !strings.Contains(h.Body, "appears to use Next.js") {
		t.Errorf("resolution lost the body: %q", h.Body)
	}
}

func TestParseMetaCoercesNonStringScalars(t *testing.T) {
	// An unquoted numeric id and an unquoted timestamp decode to int and
	// time.Time respectively; both must still come back as text.
	m := ParseMeta([]byte("---\nrex_handoff_id: 838301\nto: alice\ncreated_at: 2026-08-22T17:15:00-04:00\n---\n\nbody\n"))
	if m.ID != "838301" {
		t.Errorf("ID = %q, want 838301", m.ID)
	}
	if !strings.HasPrefix(m.CreatedAt, "2026-08-22T17:15:00") {
		t.Errorf("CreatedAt = %q", m.CreatedAt)
	}
}

func TestNewIDIsSixHexChars(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id := NewID()
		if len(id) != 6 {
			t.Fatalf("NewID() = %q, want 6 hex chars", id)
		}
		if strings.ContainsFunc(id, func(r rune) bool {
			return !strings.ContainsRune("0123456789abcdef", r)
		}) {
			t.Fatalf("NewID() = %q, want hex", id)
		}
		seen[id] = true
	}
	if len(seen) < 40 {
		t.Errorf("NewID looks non-random: %d unique of 50", len(seen))
	}
}

func TestPathForAndAccessors(t *testing.T) {
	p := PathFor("shriyanss", "nextjs")
	if p != "handoffs/shriyanss/nextjs.md" {
		t.Fatalf("PathFor = %q", p)
	}
	if PathFor("shriyanss", "nextjs.md") != p {
		t.Error("PathFor should tolerate a title that already carries .md")
	}
	if AssigneeFromPath(p) != "shriyanss" || TitleFromPath(p) != "nextjs" {
		t.Errorf("accessors disagree with %q", p)
	}
}
