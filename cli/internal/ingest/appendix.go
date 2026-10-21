package ingest

import (
	"fmt"
	"strings"
)

// AppendixPath is the path (relative to the report directory) of the generated
// asset-inventory appendix, both when writing it and when registering it in index.md.
const AppendixPath = "appendices/assets.md"

// renderAppendixMarkdown regenerates the asset-inventory appendix in full from the
// merged asset list. It's always overwritten wholesale — assets.json, not this file,
// is the thing that accumulates across ingest runs.
//
// The inventory table itself is left to the :::asset_graph:table:ports::: directive
// (see charts.RenderAssets/_asset_matrix) rather than hand-built here: a raw markdown
// pipe table's column widths come from the Lua filter's Table() sizing columns by the
// separator row's dash count, not actual content, which overlapped/truncated on IP
// addresses and version strings.
func renderAppendixMarkdown(assets []Asset) string {
	openPorts := 0
	for _, a := range assets {
		openPorts += len(a.Ports)
	}

	var b strings.Builder
	b.WriteString("---\ntitle: \"Appendix — Discovered Assets\"\n---\n\n")
	fmt.Fprintf(&b, "%d host(s) scanned, %d open port(s) discovered.\n\n", len(assets), openPorts)

	b.WriteString("## Asset Inventory\n\n:::asset_graph:table:ports:::\n\n")
	b.WriteString("## Open Ports\n\n:::asset_graph:bar:ports:::\n\n")
	b.WriteString("## Operating Systems\n\n:::asset_graph:pie:os:::\n")

	return b.String()
}
