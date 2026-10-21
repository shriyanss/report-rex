package synccommon

import "testing"

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"findings/SQLI-001.md", "findings/SQLI-001.md", false},
		{"./findings/SQLI-001.md", "findings/SQLI-001.md", false},
		{"findings//SQLI-001.md", "findings/SQLI-001.md", false},
		{"findings\\SQLI-001.md", "findings/SQLI-001.md", false},
		{"", "", true},
		{".", "", true},
		{"/etc/passwd", "", true},
		{"../../etc/passwd", "", true},
		{"findings/../../etc/passwd", "", true},
		{"..", "", true},
		{"findings/..", "", true},
	}

	for _, c := range cases {
		got, err := NormalizePath(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("NormalizePath(%q) = %q, want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizePath(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
