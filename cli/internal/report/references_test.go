package report

import "testing"

func TestOwaspName(t *testing.T) {
	name, ok := OwaspName("A03:2021")
	if !ok || name == "" {
		t.Fatalf("OwaspName(A03:2021) = %q, %v", name, ok)
	}
}

func TestMitreName(t *testing.T) {
	name, ok := MitreName("T1190")
	if !ok || name == "" {
		t.Fatalf("MitreName(T1190) = %q, %v", name, ok)
	}
}

func TestLabelFallsBackToCode(t *testing.T) {
	if got := Label("UNKNOWN-CODE"); got != "UNKNOWN-CODE" {
		t.Errorf("Label(UNKNOWN-CODE) = %q", got)
	}
}

func TestNormaliseBusinessRisk(t *testing.T) {
	cat, ok := NormaliseBusinessRisk("Operational")
	if !ok || cat != "Operational" {
		t.Errorf("got %q, %v", cat, ok)
	}
}

func TestSplitCodesString(t *testing.T) {
	got := SplitCodes("A03, A04;A05")
	want := []string{"A03", "A04", "A05"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestSplitCodesList(t *testing.T) {
	got := SplitCodes([]any{"A03", "A04"})
	if len(got) != 2 || got[0] != "A03" || got[1] != "A04" {
		t.Errorf("got %v", got)
	}
}

func TestSplitCodesNil(t *testing.T) {
	if got := SplitCodes(nil); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}
