-- Runs `rex report validate --dir <dir> --json` and surfaces issues as
-- vim.diagnostic entries (issue #26). One-shot per run -- report validate
-- exits after a single invocation, unlike bridge.lua's long-lived NDJSON
-- stream -- so there's no persistent job to track between runs.
local M = {}

M.enabled = true
M.override = "auto" -- "auto" | "on" | "off"

local ns = vim.api.nvim_create_namespace("rex_presence_lint")

-- is_rex_project checks for index.md, the same sole "is this a report"
-- signal cli/cmd/report_validate.go itself uses.
function M.is_rex_project(dir)
	return dir ~= nil and vim.loop.fs_stat(dir .. "/index.md") ~= nil
end

-- effective_enabled combines the runtime toggle (M.enabled) with the manual
-- project-type override (M.override): M.enabled is the master switch, "off"
-- forces linting off regardless of detection, "on" skips auto-detection,
-- "auto" (default) defers to whether index.md was actually found.
function M.effective_enabled(dir)
	if not M.enabled then
		return false
	end
	if M.override == "off" then
		return false
	end
	if M.override == "on" then
		return true
	end
	return M.is_rex_project(dir)
end

-- parse_output decodes `rex report validate --json`'s stdout -- a single
-- JSON object, not NDJSON. Returns nil for empty/malformed output; linting
-- is advisory, a corrupt run just means diagnostics aren't updated rather
-- than an error being raised (mirrors bridge.lua's parse_line).
function M.parse_output(stdout)
	if not stdout or stdout:match("^%s*$") then
		return nil
	end
	local ok, decoded = pcall(vim.json.decode, stdout)
	if not ok or type(decoded) ~= "table" then
		return nil
	end
	if decoded.error then
		return { error = decoded.error }
	end
	if type(decoded.issues) == "table" then
		return decoded
	end
	return nil
end

local SEVERITY = {
	error = vim.diagnostic.severity.ERROR,
	warning = vim.diagnostic.severity.WARN,
}

-- issues_to_diagnostics buckets issues by file into vim.diagnostic entries.
-- An issue has no line/column -- rex report validate reports per-file, not
-- per-line -- so every diagnostic spans line 0 (lnum is 0-indexed here) of
-- the file it names, a known limitation of the CLI, not faked here.
function M.issues_to_diagnostics(issues)
	local by_file = {}
	for _, issue in ipairs(issues) do
		by_file[issue.file] = by_file[issue.file] or {}
		table.insert(by_file[issue.file], {
			lnum = 0,
			col = 0,
			message = string.format("%s (%s)", issue.message, issue.check),
			severity = SEVERITY[issue.severity] or vim.diagnostic.severity.WARN,
			source = "rex report validate",
		})
	end
	return by_file
end

-- relative_to_dir returns bufname's path relative to dir (both resolved to
-- absolute first), or nil if bufname isn't under dir -- anchored to the same
-- --dir argument passed to `rex report validate`, not Neovim's cwd, so it
-- stays correct even if they differ.
function M.relative_to_dir(bufname, dir)
	if not bufname or bufname == "" or not dir then
		return nil
	end
	local abs = vim.fn.fnamemodify(bufname, ":p")
	local dir_abs = vim.fn.fnamemodify(dir, ":p")
	if dir_abs:sub(-1) ~= "/" then
		dir_abs = dir_abs .. "/"
	end
	if abs:sub(1, #dir_abs) ~= dir_abs then
		return nil
	end
	return (abs:sub(#dir_abs + 1):gsub("\\", "/"))
end

function M.clear()
	for _, bufnr in ipairs(vim.api.nvim_list_bufs()) do
		vim.diagnostic.reset(ns, bufnr)
	end
end

-- apply sets diagnostics on every currently loaded buffer whose path
-- resolves (relative to dir) to one of issues' files.
function M.apply(issues, dir)
	local by_file = M.issues_to_diagnostics(issues)
	for _, bufnr in ipairs(vim.api.nvim_list_bufs()) do
		if vim.api.nvim_buf_is_loaded(bufnr) then
			local rel = M.relative_to_dir(vim.api.nvim_buf_get_name(bufnr), dir)
			if rel and by_file[rel] then
				vim.diagnostic.set(ns, bufnr, by_file[rel])
			end
		end
	end
end

-- run spawns `<bin> report validate --dir <dir> --json` once and refreshes
-- diagnostics from its result -- clearing them entirely when linting isn't
-- currently active, or when the run fails/exits with unparseable output
-- (never blocks on error, matches presence/sync's fail-open behavior).
function M.run(dir, bin)
	if not M.effective_enabled(dir) then
		M.clear()
		return
	end
	vim.fn.jobstart({ bin or "rex", "report", "validate", "--dir", dir, "--json" }, {
		stdout_buffered = true,
		on_stdout = function(_, data)
			local result = M.parse_output(table.concat(data or {}, "\n"))
			M.clear()
			if not result or result.error then
				return
			end
			M.apply(result.issues, dir)
		end,
	})
end

function M.toggle(dir, bin)
	M.enabled = not M.enabled
	vim.notify("rex-presence: linter " .. (M.enabled and "enabled" or "disabled"), vim.log.levels.INFO)
	if M.enabled then
		M.run(dir, bin)
	else
		M.clear()
	end
end

function M.set_override(value, dir, bin)
	if value ~= "auto" and value ~= "on" and value ~= "off" then
		vim.notify(
			"rex-presence: invalid project override " .. tostring(value) .. " (expected auto|on|off)",
			vim.log.levels.ERROR
		)
		return
	end
	M.override = value
	M.run(dir, bin)
end

function M.setup(opts)
	opts = opts or {}
	if opts.enabled ~= nil then
		M.enabled = opts.enabled
	end
	if opts.override then
		M.override = opts.override
	end
end

return M
