// Package cvss parses CVSS v3.x/v4.0 vector strings and computes the base
// score and qualitative severity. Ported from
// core/src/report_rex_core/cvss.py + cvss4.py.
package cvss

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// SeverityRank is the numeric rank used for sorting findings (higher = more severe).
var SeverityRank = map[string]int{
	"Critical": 4, "High": 3, "Medium": 2, "Low": 1, "None": 0, "Info": 0,
}

var av = map[string]float64{"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.20}
var ac = map[string]float64{"L": 0.77, "H": 0.44}
var ui = map[string]float64{"N": 0.85, "R": 0.62}
var cia = map[string]float64{"H": 0.56, "L": 0.22, "N": 0.00}
var prUnchanged = map[string]float64{"N": 0.85, "L": 0.62, "H": 0.27}
var prChanged = map[string]float64{"N": 0.85, "L": 0.68, "H": 0.50}

var requiredV3 = []string{"AV", "AC", "PR", "UI", "S", "C", "I", "A"}

// Result is the outcome of a successful Compute call.
type Result struct {
	Version  string
	Score    float64
	Severity string
	Vector   string
}

// ParseVector splits a CVSS vector into (version, metric->value). Accepts an
// optional "CVSS:x.y/" prefix, defaulting to v3.1. Returns an error on
// malformed input or missing base metrics.
func ParseVector(vector string) (string, map[string]string, error) {
	if strings.TrimSpace(vector) == "" {
		return "", nil, fmt.Errorf("empty CVSS vector")
	}

	version := "3.1"
	metrics := map[string]string{}
	for _, part := range strings.Split(strings.TrimSpace(vector), "/") {
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, ":")
		if !ok {
			return "", nil, fmt.Errorf("malformed CVSS component: %q", part)
		}
		key = strings.ToUpper(strings.TrimSpace(key))
		value = strings.ToUpper(strings.TrimSpace(value))
		if key == "CVSS" {
			version = value
			continue
		}
		metrics[key] = value
	}

	required := requiredV3
	if strings.SplitN(version, ".", 2)[0] == "4" {
		required = BaseMetrics
	}
	var missing []string
	for _, m := range required {
		if _, ok := metrics[m]; !ok {
			missing = append(missing, m)
		}
	}
	if len(missing) > 0 {
		return "", nil, fmt.Errorf("CVSS vector missing base metrics: %s", strings.Join(missing, ", "))
	}

	return version, metrics, nil
}

func roundup(value float64, version string) float64 {
	if version == "3.0" {
		return math.Ceil(value*10) / 10
	}
	// CVSS v3.1 official Roundup (integer-arithmetic definition).
	intInput := int64(math.Round(value * 100_000))
	if intInput%10_000 == 0 {
		return float64(intInput) / 100_000
	}
	return float64(intInput/10_000+1) / 10
}

// BaseScore computes the CVSS base score for a vector string, returning
// (version, score).
func BaseScore(vector string) (string, float64, error) {
	version, m, err := ParseVector(vector)
	if err != nil {
		return "", 0, err
	}
	major := strings.SplitN(version, ".", 2)[0]
	if major == "4" {
		score, err := baseScoreV4(m)
		return version, score, err
	}
	if major != "3" {
		return "", 0, fmt.Errorf("base-score computation only supports CVSS v3.x and v4.0 (got %s)", version)
	}

	scopeChanged := m["S"] == "C"
	avv, ok1 := av[m["AV"]]
	acv, ok2 := ac[m["AC"]]
	uiv, ok3 := ui[m["UI"]]
	prTable := prUnchanged
	if scopeChanged {
		prTable = prChanged
	}
	prv, ok4 := prTable[m["PR"]]
	c, ok5 := cia[m["C"]]
	i, ok6 := cia[m["I"]]
	a, ok7 := cia[m["A"]]
	if !(ok1 && ok2 && ok3 && ok4 && ok5 && ok6 && ok7) {
		return "", 0, fmt.Errorf("invalid CVSS metric value")
	}

	iss := 1 - (1-c)*(1-i)*(1-a)
	var impact float64
	if scopeChanged {
		impact = 7.52*(iss-0.029) - 3.25*math.Pow(iss-0.02, 15)
	} else {
		impact = 6.42 * iss
	}

	exploitability := 8.22 * avv * acv * prv * uiv

	var score float64
	if impact <= 0 {
		score = 0.0
	} else if scopeChanged {
		score = roundup(math.Min(1.08*(impact+exploitability), 10), version)
	} else {
		score = roundup(math.Min(impact+exploitability, 10), version)
	}

	return version, roundHalfAway(score, 1), nil
}

func baseScoreV4(metrics map[string]string) (float64, error) {
	for key, value := range metrics {
		allowed, ok := MetricValues[key]
		if !ok {
			continue
		}
		found := false
		for _, a := range allowed {
			if a == value {
				found = true
				break
			}
		}
		if !found {
			return 0, fmt.Errorf("invalid CVSS metric value: %s:%s", key, value)
		}
	}

	selected := map[string]string{}
	keys := make([]string, 0, len(MetricValues))
	for k := range MetricValues {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if v, ok := metrics[key]; ok {
			selected[key] = v
		} else {
			selected[key] = "X"
		}
	}
	return BaseScoreV4(selected), nil
}

// roundHalfAway rounds to the given number of decimal places, half away from zero.
func roundHalfAway(value float64, decimals int) float64 {
	shift := math.Pow(10, float64(decimals))
	if value >= 0 {
		return math.Floor(value*shift+0.5) / shift
	}
	return math.Ceil(value*shift-0.5) / shift
}

// SeverityFromScore maps a numeric base score to the CVSS qualitative severity rating.
func SeverityFromScore(score float64) string {
	switch {
	case score <= 0:
		return "None"
	case score < 4.0:
		return "Low"
	case score < 7.0:
		return "Medium"
	case score < 9.0:
		return "High"
	default:
		return "Critical"
	}
}

// Compute parses a vector and returns its version, score, and qualitative severity.
func Compute(vector string) (Result, error) {
	version, score, err := BaseScore(vector)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Version:  version,
		Score:    score,
		Severity: SeverityFromScore(score),
		Vector:   strings.TrimSpace(vector),
	}, nil
}
