-- Manual self-check for lint.lua's pure logic (no jobstart, no real `rex
-- report validate` process). Not wired into CI -- there's no Neovim in the
-- CI image -- run it locally after touching lint.lua:
--
--   nvim --headless -l cli/editors/nvim/lua/rex-presence/lint_spec.lua
--
-- Exits 0 and prints "ok" on success, non-zero with a message on failure.
package.path = vim.fn.getcwd() .. "/lua/?.lua;" .. package.path

local lint = require("rex-presence.lint")

local function check(name, cond)
	if not cond then
		io.stderr:write("FAIL: " .. name .. "\n")
		os.exit(1)
	end
end

-- parse_output
do
	local result = lint.parse_output(
		'{"issues":[{"file":"reports/vuln-1.md","check":"cvss_missing","message":"No CVSS data.","severity":"error"}],"errors":1,"warnings":0}'
	)
	check("parse_output decodes a valid issues envelope", result ~= nil and #result.issues == 1 and result.errors == 1)
end
check("parse_output decodes an empty issues envelope", (function()
	local result = lint.parse_output('{"issues":[],"errors":0,"warnings":0}')
	return result ~= nil and #result.issues == 0
end)())
check("parse_output decodes the fatal-error shape", (function()
	local result = lint.parse_output('{"error":"index.md not found in ."}')
	return result ~= nil and result.error == "index.md not found in ."
end)())
check("parse_output returns nil for malformed/empty output", lint.parse_output("not json") == nil)
check("parse_output returns nil for blank output", lint.parse_output("   \n  ") == nil and lint.parse_output(nil) == nil)
check("parse_output returns nil for unexpected shape", lint.parse_output('{"unexpected":true}') == nil)

-- issues_to_diagnostics
do
	local by_file = lint.issues_to_diagnostics({
		{ file = "reports/vuln-1.md", check = "cvss_missing", message = "No CVSS data.", severity = "error" },
		{ file = "reports/vuln-1.md", check = "timeline_missing", message = "No timeline.", severity = "error" },
		{ file = "reports/vuln-2.md", check = "screenshot_caption_length", message = "Too short.", severity = "warning" },
	})
	check("issues_to_diagnostics groups by file", #by_file["reports/vuln-1.md"] == 2 and #by_file["reports/vuln-2.md"] == 1)
	check("issues_to_diagnostics maps error severity", by_file["reports/vuln-1.md"][1].severity == vim.diagnostic.severity.ERROR)
	check(
		"issues_to_diagnostics maps warning severity",
		by_file["reports/vuln-2.md"][1].severity == vim.diagnostic.severity.WARN
	)
	check("issues_to_diagnostics defaults lnum/col to 0", by_file["reports/vuln-1.md"][1].lnum == 0 and by_file["reports/vuln-1.md"][1].col == 0)
end

-- relative_to_dir
check("relative_to_dir strips the dir prefix", lint.relative_to_dir("/repo/report/reports/vuln-1.md", "/repo/report") == "reports/vuln-1.md")
check("relative_to_dir returns nil outside dir", lint.relative_to_dir("/elsewhere/x.md", "/repo/report") == nil)
check("relative_to_dir returns nil for an empty bufname", lint.relative_to_dir("", "/repo/report") == nil)

-- effective_enabled / override precedence
do
	lint.enabled = true
	lint.override = "off"
	check("effective_enabled: override off wins even if is_rex_project would be true", lint.effective_enabled("/nonexistent") == false)

	lint.override = "on"
	check("effective_enabled: override on skips detection", lint.effective_enabled("/nonexistent") == true)

	lint.enabled = false
	lint.override = "on"
	check("effective_enabled: M.enabled is the master switch", lint.effective_enabled("/nonexistent") == false)

	lint.enabled = true
	lint.override = "auto"
	check("effective_enabled: auto defers to is_rex_project (no index.md here)", lint.effective_enabled("/nonexistent") == false)
end

print("ok")
