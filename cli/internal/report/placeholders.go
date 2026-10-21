package report

// Scaffold sentinel strings, ported verbatim from builder.py's PLACEHOLDER_*
// constants — the single source of truth for leftover-placeholder-text
// detection in validate.go, regardless of which scaffolder (Python or Go)
// produced the report.
const (
	PlaceholderVulnTitle      = "Example Vulnerability"
	PlaceholderDescription    = "Describe the vulnerability here."
	PlaceholderImpact         = "Describe the business and technical impact."
	PlaceholderRecommendation = "Describe remediation steps. Include code examples where helpful."
	PlaceholderMethodology    = "Describe the testing methodology, scope, and approach used during the engagement."
	PlaceholderContactName    = "Your Name"
	PlaceholderContactEmail   = "you@example.com"
	// Substring match catches both "1970-01-01" (date/start_date/end_date/
	// retest_date) and "1970-01-01T00:00:00+00:00" (timeline entries) — the
	// Unix epoch, a date no real engagement can have, used as the "noticeable"
	// scaffold/example date.
	PlaceholderEpochDate = "1970-01-01"
	// Umbrella sentinel embedded throughout the rewritten reporting/example
	// content (see reporting/CLAUDE.md) so an unedited copy is caught here too.
	PlaceholderLoremIpsum = "Lorem Ipsum"
	// Template defaults in the scaffolds' index.md that render on the cover /
	// Document Control page (#74): the example's report_id and ACME client
	// logo, and `report-rex new`'s report_id.
	PlaceholderExampleReportID = "RR-2024-0142"
	PlaceholderNewReportID     = "RR-2024-0001"
	PlaceholderClientLogo      = "acme-logo.png"
)

// PlaceholderScreenshotSHA256 is the SHA-256 of the generated placeholder
// screenshot shipped at reporting/example/assets/screenshots/*.png (see
// scripts/gen_placeholder_screenshot.py). A var, not a const, so tests can
// swap it for a fixture hash.
var PlaceholderScreenshotSHA256 = "f38bed7f262ca07135c0ed47f7eb128f2d6e813a686983c5c8ce15034bd0ee9f"

var placeholderStrings = []string{
	PlaceholderVulnTitle,
	PlaceholderDescription,
	PlaceholderImpact,
	PlaceholderRecommendation,
	PlaceholderMethodology,
	PlaceholderContactName,
	PlaceholderContactEmail,
	PlaceholderEpochDate,
	PlaceholderLoremIpsum,
	PlaceholderExampleReportID,
	PlaceholderNewReportID,
	PlaceholderClientLogo,
}
