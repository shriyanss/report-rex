// Package buildscript defines the flags shared by `report-rex report build`, mirroring
// build.sh's flag set so the Go command is a drop-in replacement (except --dir, which
// defaults to the current directory here instead of build.sh's reporting/example).
package buildscript

import "path/filepath"

// BuildFlags mirrors build.sh's CLI surface.
type BuildFlags struct {
	Dir      string // report directory (default: cwd)
	Template string // template override; empty means "use index.md's template"
	Output   string // output PDF filename (default: report.pdf)
	Final    bool   // produce the final report (no DRAFT watermark, plain filename); default is draft
	Docx     bool   // also generate a .docx clone alongside the PDF
	Force    bool   // skip QA-gate validation
	NoDocker bool   // invoke the local report-rex Python CLI instead of Docker
	Image    string // Docker image tag (default: report-rex:<version>)
	Password string // if set, encrypt the output PDF with this password (via qpdf)

	NoTerminalBS bool // suppress the build progress indicator
}

// OutputFile returns the effective output filename, applying the -draft suffix
// build.sh applies unless --final is set.
func (f BuildFlags) OutputFile() string {
	if f.Final {
		return f.Output
	}
	ext := ".pdf"
	name := f.Output
	if len(name) > len(ext) && name[len(name)-len(ext):] == ext {
		name = name[:len(name)-len(ext)]
	}
	return name + "-draft" + ext
}

// OutputDocxFile returns the DOCX sibling path (swapping the extension for
// .docx), derived from the raw --output value rather than OutputFile()'s
// -draft-suffixed name — the DOCX clone isn't watermarked, so it doesn't need
// the suffix.
func (f BuildFlags) OutputDocxFile() string {
	ext := filepath.Ext(f.Output)
	return f.Output[:len(f.Output)-len(ext)] + ".docx"
}
