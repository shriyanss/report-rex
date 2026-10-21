package ingest

import (
	"fmt"
	"sort"
	"strings"
)

// ADAppendixPath is the path (relative to the report directory) of the generated
// AD-enumeration appendix `rex ingest bloodhound` writes.
const ADAppendixPath = "appendices/ad-enum.md"

// renderADAppendixMarkdown regenerates the AD-enumeration appendix in full from the
// merged AD dataset, mirroring the other generated appendices' always-overwrite
// approach. Rather than reproducing BloodHound's own graph-based attack-path
// analysis (out of scope for a report appendix), it surfaces the handful of raw AD
// attributes pentesters flag directly off SharpHound's own collected properties:
// admincount (AdminSDHolder-protected, i.e. historically or currently privileged),
// hasspn (Kerberoastable), dontreqpreauth (AS-REP roastable), and
// unconstraineddelegation.
func renderADAppendixMarkdown(data ADData) string {
	domains := append([]string{}, data.Domains...)
	sort.Strings(domains)

	principals := append([]ADPrincipal{}, data.Principals...)
	sort.Slice(principals, func(i, j int) bool {
		if principals[i].Domain != principals[j].Domain {
			return principals[i].Domain < principals[j].Domain
		}
		return principals[i].Name < principals[j].Name
	})

	counts := map[string]int{}
	for _, p := range principals {
		counts[p.Type]++
	}

	var b strings.Builder
	b.WriteString("---\ntitle: \"Appendix — Active Directory Enumeration\"\n---\n\n")

	if len(domains) > 0 {
		fmt.Fprintf(&b, "Domains collected: %s.\n\n", strings.Join(domains, ", "))
	}

	b.WriteString("## Object Counts\n\n")
	b.WriteString("| Type | Count |\n|------|-------|\n")
	for _, t := range []string{"user", "computer", "group"} {
		fmt.Fprintf(&b, "| %ss | %d |\n", t, counts[t])
	}

	writeSection := func(title, emptyNote string, match func(ADPrincipal) bool) {
		b.WriteString("\n## " + title + "\n\n")
		rows := 0
		for _, p := range principals {
			if !match(p) {
				continue
			}
			fmt.Fprintf(&b, "- `%s` (%s, %s)\n", p.Name, p.Domain, p.Type)
			rows++
		}
		if rows == 0 {
			b.WriteString(emptyNote + "\n")
		}
	}

	writeSection("High-Value Principals (AdminSDHolder-protected)", "None found.", func(p ADPrincipal) bool {
		return p.AdminCount
	})
	writeSection("Kerberoastable Accounts", "None found.", func(p ADPrincipal) bool {
		return p.Type == "user" && p.Enabled && p.HasSPN
	})
	writeSection("AS-REP Roastable Accounts", "None found.", func(p ADPrincipal) bool {
		return p.Type == "user" && p.Enabled && p.DontReqPreauth
	})
	writeSection("Unconstrained Delegation", "None found.", func(p ADPrincipal) bool {
		return p.Type == "computer" && p.UnconstrainedDelegation
	})

	return b.String()
}
