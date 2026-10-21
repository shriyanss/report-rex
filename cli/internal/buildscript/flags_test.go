package buildscript

import "testing"

func TestOutputFile(t *testing.T) {
	cases := []struct {
		name  string
		flags BuildFlags
		want  string
	}{
		{"final", BuildFlags{Output: "report.pdf", Final: true}, "report.pdf"},
		{"draft (default)", BuildFlags{Output: "report.pdf", Final: false}, "report-draft.pdf"},
		{"draft non-pdf ext", BuildFlags{Output: "report", Final: false}, "report-draft.pdf"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.flags.OutputFile(); got != c.want {
				t.Errorf("OutputFile() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestOutputDocxFile(t *testing.T) {
	cases := []struct {
		name  string
		flags BuildFlags
		want  string
	}{
		{"swaps extension", BuildFlags{Output: "report.pdf"}, "report.docx"},
		{"no extension", BuildFlags{Output: "report"}, "report.docx"},
		{"draft does not affect docx name", BuildFlags{Output: "report.pdf", Final: false}, "report.docx"},
		{"custom output path", BuildFlags{Output: "out/my-report.pdf"}, "out/my-report.docx"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.flags.OutputDocxFile(); got != c.want {
				t.Errorf("OutputDocxFile() = %q, want %q", got, c.want)
			}
		})
	}
}
