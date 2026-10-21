package docker

import (
	"fmt"
	"testing"
)

func TestFormatOCR(t *testing.T) {
	raw := "\nGET / HTTP/1.1   \nHost: x\n\n\n\n    indented\t\n\f"
	want := "```http\nGET / HTTP/1.1\nHost: x\n\n    indented\n```\n"
	if got := FormatOCR(raw, "http"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLayoutTSV(t *testing.T) {
	// 10px-wide monospace glyphs, 20px lines; line 3 is indented 4 columns
	// and preceded by a blank line (40px gap).
	row := func(line, left, top int, text string) string {
		return fmt.Sprintf("5\t1\t1\t1\t%d\t1\t%d\t%d\t%d\t14\t90\t%s\n", line, left, top, 10*len([]rune(text)), text)
	}
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		row(1, 10, 10, "def") + row(1, 50, 10, "f():") +
		row(2, 10, 30, "x") + row(2, 50, 30, "=") +
		row(4, 10, 90, "{\"a\":") + row(4, 60, 90, "1}") +
		row(3, 50, 70, "return") + row(3, 120, 70, "“ok”")
	want := "def f():\nx   =\n\n    return \"ok\"\n{\"a\":1}\n"
	if got := LayoutTSV(tsv); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
