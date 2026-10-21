package cvss

import "testing"

func TestKnownVectors(t *testing.T) {
	cases := []struct {
		vector   string
		score    float64
		severity string
	}{
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", 9.8, "Critical"},
		{"AV:N/AC:L/PR:L/UI:R/S:C/C:H/I:H/A:N", 8.7, "High"},
		{"AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:N/A:N", 6.5, "Medium"},
		{"AV:N/AC:H/PR:N/UI:R/S:U/C:L/I:N/A:N", 3.1, "Low"},
		{"AV:P/AC:H/PR:H/UI:R/S:U/C:N/I:N/A:N", 0.0, "None"},
	}
	for _, c := range cases {
		result, err := Compute(c.vector)
		if err != nil {
			t.Fatalf("Compute(%q) error: %v", c.vector, err)
		}
		if result.Score != c.score || result.Severity != c.severity {
			t.Errorf("Compute(%q) = (%v, %v), want (%v, %v)", c.vector, result.Score, result.Severity, c.score, c.severity)
		}
	}
}

func TestDefaultsToV31WithoutPrefix(t *testing.T) {
	result, err := Compute("AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H")
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != "3.1" {
		t.Errorf("got version %q, want 3.1", result.Version)
	}
}

func TestReadsVersionPrefix(t *testing.T) {
	result, err := Compute("CVSS:3.0/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H")
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != "3.0" {
		t.Errorf("got version %q, want 3.0", result.Version)
	}
}

func TestMissingMetricErrors(t *testing.T) {
	if _, err := Compute("AV:N/AC:L"); err == nil {
		t.Error("expected error for missing metrics")
	}
}

func TestKnownV4Vectors(t *testing.T) {
	cases := []struct {
		vector   string
		score    float64
		severity string
	}{
		{"CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N", 9.3, "Critical"},
		{"CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:H/SI:H/SA:H", 10.0, "Critical"},
		{"CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:L/VI:L/VA:L/SC:N/SI:N/SA:N", 6.9, "Medium"},
		{"CVSS:4.0/AV:N/AC:L/AT:N/PR:L/UI:N/VC:L/VI:L/VA:N/SC:N/SI:N/SA:N", 5.3, "Medium"},
		{"CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:P/VC:H/VI:N/VA:N/SC:N/SI:N/SA:N", 7.1, "High"},
		{"CVSS:4.0/AV:P/AC:H/AT:P/PR:H/UI:A/VC:N/VI:N/VA:N/SC:N/SI:N/SA:N", 0.0, "None"},
	}
	for _, c := range cases {
		result, err := Compute(c.vector)
		if err != nil {
			t.Fatalf("Compute(%q) error: %v", c.vector, err)
		}
		if result.Score != c.score || result.Severity != c.severity || result.Version != "4.0" {
			t.Errorf("Compute(%q) = (%v, %v, %v), want (%v, %v, 4.0)", c.vector, result.Score, result.Severity, result.Version, c.score, c.severity)
		}
	}
}

func TestV4MissingBaseMetricErrors(t *testing.T) {
	if _, err := Compute("CVSS:4.0/AV:N/AC:L/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"); err == nil {
		t.Error("expected error for missing AT base metric")
	}
}

func TestV4InvalidMetricValueErrors(t *testing.T) {
	if _, err := Compute("CVSS:4.0/AV:X/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"); err == nil {
		t.Error("expected error for invalid AV value")
	}
}

func TestV4EnvironmentalMetricsAffectScore(t *testing.T) {
	base := "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:L/VA:N/SC:N/SI:N/SA:N"
	unmodified, err := Compute(base)
	if err != nil {
		t.Fatal(err)
	}
	lowRequirements, err := Compute(base + "/CR:L/IR:L/AR:L")
	if err != nil {
		t.Fatal(err)
	}
	if !(lowRequirements.Score < unmodified.Score) {
		t.Errorf("expected lowered score, got %v >= %v", lowRequirements.Score, unmodified.Score)
	}
}

func TestUnsupportedVersionErrors(t *testing.T) {
	if _, _, err := BaseScore("CVSS:2.0/AV:N/AC:L/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"); err == nil {
		t.Error("expected error for unsupported version")
	}
}

func TestSeverityBoundaries(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0.0, "None"}, {3.9, "Low"}, {4.0, "Medium"}, {6.9, "Medium"}, {7.0, "High"}, {9.0, "Critical"},
	}
	for _, c := range cases {
		if got := SeverityFromScore(c.score); got != c.want {
			t.Errorf("SeverityFromScore(%v) = %q, want %q", c.score, got, c.want)
		}
	}
}
