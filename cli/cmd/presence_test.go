package cmd

import (
	"encoding/json"
	"testing"
)

func TestPresenceInParsesUpdateAndRelease(t *testing.T) {
	var upd presenceIn
	if err := json.Unmarshal([]byte(`{"op":"update","file":"a.md","state":"editing","dirty":true}`), &upd); err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}
	if upd.Op != "update" || upd.File != "a.md" || upd.State != "editing" || !upd.Dirty {
		t.Errorf("parsed = %+v", upd)
	}

	var rel presenceIn
	if err := json.Unmarshal([]byte(`{"op":"release","file":"a.md"}`), &rel); err != nil {
		t.Fatalf("unmarshal release: %v", err)
	}
	if rel.Op != "release" || rel.File != "a.md" {
		t.Errorf("parsed = %+v", rel)
	}
}

func TestPresenceInIgnoresMalformedJSON(t *testing.T) {
	var in presenceIn
	if err := json.Unmarshal([]byte(`not json`), &in); err == nil {
		t.Fatal("expected an error unmarshaling malformed input")
	}
}

func TestPresenceOutEncodesPresenceAndStatusEvents(t *testing.T) {
	b, err := json.Marshal(presenceOut{
		Type: "presence", Username: "bob", SessionID: "s1", File: "a.md",
		State: "editing", Dirty: true, Action: "active",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out presenceOut
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if out.Type != "presence" || out.Username != "bob" || out.SessionID != "s1" ||
		out.File != "a.md" || out.State != "editing" || !out.Dirty || out.Action != "active" {
		t.Errorf("round-tripped = %+v", out)
	}

	sb, err := json.Marshal(presenceOut{Type: "status", Connected: false, Error: "boom"})
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	var status presenceOut
	if err := json.Unmarshal(sb, &status); err != nil {
		t.Fatalf("round-trip status: %v", err)
	}
	if status.Type != "status" || status.Connected || status.Error != "boom" {
		t.Errorf("round-tripped status = %+v", status)
	}
}

func TestNewSessionIDIsUniqueAndNonEmpty(t *testing.T) {
	a := newSessionID()
	b := newSessionID()
	if a == "" || b == "" {
		t.Fatal("newSessionID() returned an empty string")
	}
	if a == b {
		t.Fatal("newSessionID() returned the same value twice")
	}
}
