package synccommon

import "testing"

func TestClassify(t *testing.T) {
	exists := func(h string) PathSnapshot { return PathSnapshot{Exists: true, Hash: h} }
	absent := PathSnapshot{}

	cases := []struct {
		name        string
		hasLastSync bool
		lastSynced  PathSnapshot
		local       PathSnapshot
		remote      PathSnapshot
		want        State
	}{
		{"unchanged", true, exists("a"), exists("a"), exists("a"), StateUnchanged},
		{"local only changed", true, exists("a"), exists("b"), exists("a"), StateLocalOnlyChanged},
		{"remote only changed", true, exists("a"), exists("a"), exists("b"), StateRemoteOnlyChanged},
		{"both changed divergently", true, exists("a"), exists("b"), exists("c"), StateBothChanged},
		{"both changed convergently", true, exists("a"), exists("b"), exists("b"), StateUnchanged},
		{"local deleted", true, exists("a"), absent, exists("a"), StateLocalOnlyChanged},
		{"remote deleted", true, exists("a"), exists("a"), absent, StateRemoteOnlyChanged},
		{"both deleted", true, exists("a"), absent, absent, StateUnchanged},
		// Initial sync: remote empty, local populated -> push, not a conflict.
		{"initial: remote empty, local populated", false, absent, exists("a"), absent, StateLocalOnlyChanged},
		// Initial sync: remote populated, local empty -> pull, not a conflict.
		{"initial: remote populated, local empty", false, absent, absent, exists("a"), StateRemoteOnlyChanged},
		// Initial sync: both identical.
		{"initial: both identical", false, absent, exists("a"), exists("a"), StateUnchanged},
		// Initial sync: both absent.
		{"initial: both absent", false, absent, absent, absent, StateUnchanged},
		// Initial sync: both differ, no known base.
		{"initial: both differ", false, absent, exists("a"), exists("b"), StateInitialUnknownBase},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.hasLastSync, c.lastSynced, c.local, c.remote)
			if got != c.want {
				t.Errorf("Classify() = %v, want %v", got, c.want)
			}
		})
	}
}
