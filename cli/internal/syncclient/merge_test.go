package syncclient

import "testing"

func TestThreeWayMergeCleanDisjointHunks(t *testing.T) {
	base := []byte("line1\nline2\nline3\nline4\nline5\n")
	local := []byte("line1 EDITED\nline2\nline3\nline4\nline5\n")
	remote := []byte("line1\nline2\nline3\nline4\nline5 EDITED\n")

	merged, clean := ThreeWayMerge(base, local, remote)
	if !clean {
		t.Fatalf("expected clean merge, got conflict: %s", merged)
	}
	want := "line1 EDITED\nline2\nline3\nline4\nline5 EDITED\n"
	if string(merged) != want {
		t.Errorf("merged = %q, want %q", merged, want)
	}
}

func TestThreeWayMergeLocalUnchangedRemoteWins(t *testing.T) {
	base := []byte("hello\n")
	local := []byte("hello\n")
	remote := []byte("hello world\n")

	merged, clean := ThreeWayMerge(base, local, remote)
	if !clean {
		t.Fatalf("expected clean merge")
	}
	if string(merged) != "hello world\n" {
		t.Errorf("merged = %q, want %q", merged, "hello world\n")
	}
}

func TestThreeWayMergeRemoteUnchangedLocalWins(t *testing.T) {
	base := []byte("hello\n")
	local := []byte("hello there\n")
	remote := []byte("hello\n")

	merged, clean := ThreeWayMerge(base, local, remote)
	if !clean {
		t.Fatalf("expected clean merge")
	}
	if string(merged) != "hello there\n" {
		t.Errorf("merged = %q, want %q", merged, "hello there\n")
	}
}

func TestThreeWayMergeConvergedIdenticalEdits(t *testing.T) {
	base := []byte("hello\n")
	local := []byte("hello world\n")
	remote := []byte("hello world\n")

	merged, clean := ThreeWayMerge(base, local, remote)
	if !clean {
		t.Fatalf("expected clean merge")
	}
	if string(merged) != "hello world\n" {
		t.Errorf("merged = %q, want %q", merged, "hello world\n")
	}
}

func TestThreeWayMergeOverlappingEditsConflict(t *testing.T) {
	base := []byte("The quick brown fox jumps over the lazy dog.\n")
	local := []byte("The quick brown fox leaps over the lazy dog and runs away fast.\n")
	remote := []byte("A completely different sentence about something else entirely, unrelated to foxes.\n")

	merged, clean := ThreeWayMerge(base, local, remote)
	if clean {
		t.Fatalf("expected a conflict for heavily overlapping/incompatible edits, got clean merge: %s", merged)
	}
	if !HasConflictMarkers(merged) {
		t.Errorf("expected conflict markers in merged output, got: %s", merged)
	}
	// Both original versions must be recoverable from the conflict output.
	s := string(merged)
	if !containsSubstring(s, string(local)) {
		t.Error("conflict output missing local version")
	}
	if !containsSubstring(s, string(remote)) {
		t.Error("conflict output missing remote version")
	}
}

func TestThreeWayMergeEmptyBaseNewFile(t *testing.T) {
	base := []byte("")
	local := []byte("local content\n")
	remote := []byte("remote content\n")

	merged, clean := ThreeWayMerge(base, local, remote)
	if clean {
		t.Fatalf("expected conflict when both sides independently create different content from nothing: %s", merged)
	}
	if !HasConflictMarkers(merged) {
		t.Error("expected conflict markers")
	}
}

func TestThreeWayMergePureAddition(t *testing.T) {
	base := []byte("a\nb\nc\n")
	local := []byte("a\nb\nc\nd\n")
	remote := []byte("a\nb\nc\n")

	merged, clean := ThreeWayMerge(base, local, remote)
	if !clean {
		t.Fatalf("expected clean merge for pure addition with unchanged remote")
	}
	if string(merged) != "a\nb\nc\nd\n" {
		t.Errorf("merged = %q", merged)
	}
}

func TestThreeWayMergePureDeletion(t *testing.T) {
	base := []byte("a\nb\nc\nd\n")
	local := []byte("a\nb\nc\n")
	remote := []byte("a\nb\nc\nd\n")

	merged, clean := ThreeWayMerge(base, local, remote)
	if !clean {
		t.Fatalf("expected clean merge for pure deletion with unchanged remote")
	}
	if string(merged) != "a\nb\nc\n" {
		t.Errorf("merged = %q", merged)
	}
}

func TestHasConflictMarkers(t *testing.T) {
	clean := []byte("no markers here\n")
	if HasConflictMarkers(clean) {
		t.Error("HasConflictMarkers(clean) = true, want false")
	}
	marked := conflictMarkers([]byte("a"), []byte("b"))
	if !HasConflictMarkers(marked) {
		t.Error("HasConflictMarkers(marked) = false, want true")
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return len(sub) == 0
}
