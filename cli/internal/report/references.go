package report

import (
	_ "embed"
	"regexp"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// referencesYAML is a copy of core/src/report_rex_core/data/references.yaml —
// keep the two in sync if OWASP/MITRE category names change.
//
//go:embed data/references.yaml
var referencesYAML []byte

// BusinessRiskCategories are the five standard business-risk impact categories.
var BusinessRiskCategories = []string{"Strategic", "Operational", "Financial", "Compliance", "Reputational"}

var (
	referencesOnce sync.Once
	referencesData map[string]map[string]string
)

func loadReferences() map[string]map[string]string {
	referencesOnce.Do(func() {
		var raw map[string]map[string]string
		_ = yaml.Unmarshal(referencesYAML, &raw)
		if raw == nil {
			raw = map[string]map[string]string{}
		}
		referencesData = raw
	})
	return referencesData
}

func normaliseCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// OwaspName resolves an OWASP code (web/API/LLM/mobile) to its official name.
// Accepts forms like "A03", "A03:2021", "API1", "LLM01", "M1".
func OwaspName(code string) (string, bool) {
	raw := normaliseCode(code)
	bare := strings.SplitN(raw, ":", 2)[0]
	data := loadReferences()
	for _, table := range []string{"owasp_web_2021", "owasp_api_2023", "owasp_llm_2025", "owasp_mobile_2024"} {
		for k, v := range data[table] {
			if strings.ToUpper(k) == bare {
				return v, true
			}
		}
	}
	return "", false
}

// MitreName resolves a MITRE ATT&CK technique (Txxxx) or tactic (TAxxxx) ID to its name.
func MitreName(code string) (string, bool) {
	raw := normaliseCode(code)
	data := loadReferences()
	tactics := data["mitre_tactics"]
	techniques := data["mitre_techniques"]
	for k, v := range tactics {
		if strings.ToUpper(k) == raw {
			return v, true
		}
	}
	for k, v := range techniques {
		if strings.ToUpper(k) == raw {
			return v, true
		}
	}
	parent := strings.SplitN(raw, ".", 2)[0]
	for k, v := range techniques {
		if strings.ToUpper(k) == parent {
			return v, true
		}
	}
	return "", false
}

// Label returns "CODE — Name" when a name is known, else just the code.
func Label(code string) string {
	code = strings.TrimSpace(code)
	if name, ok := OwaspName(code); ok {
		return code + " — " + name
	}
	if name, ok := MitreName(code); ok {
		return code + " — " + name
	}
	return code
}

// NormaliseBusinessRisk matches a free-text business-risk value to a canonical category.
func NormaliseBusinessRisk(value string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(value))
	for _, cat := range BusinessRiskCategories {
		lc := strings.ToLower(cat)
		if strings.HasPrefix(lc, v) || strings.HasPrefix(v, lc) {
			return cat, true
		}
	}
	return "", false
}

var codeSplitRe = regexp.MustCompile(`[,;]`)

// SplitCodes splits a frontmatter field that may be a list or a delimited string.
func SplitCodes(value any) []string {
	if value == nil {
		return nil
	}
	var items []string
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			items = append(items, toStr(item))
		}
	default:
		items = codeSplitRe.Split(toStr(value), -1)
	}
	out := make([]string, 0, len(items))
	for _, i := range items {
		i = strings.TrimSpace(i)
		if i != "" {
			out = append(out, i)
		}
	}
	return out
}
