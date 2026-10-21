-- Neovim integration for Report Rex's advisory editor presence (issue #11):
-- reports which workspace-relative file you're viewing/editing to a
-- `rex presence` bridge process, and surfaces teammates' presence on the
-- files you open. Never blocks editing -- any bridge/connection failure
-- just means no presence info is shown.
local bridge = require("rex-presence.bridge")
local statusline = require("rex-presence.statusline")
local explorer = require("rex-presence.explorer")
local syncjob = require("rex-presence.syncjob")
local lint = require("rex-presence.lint")

local M = {}

local VIEW_DELAY_MS = 1000
local HEARTBEAT_MS = 5000

local cfg = nil
local remote = {} -- remote[file][session_id] = {username=, state=, dirty=}
local tracked = {} -- tracked[bufnr] = {file=, state=, dirty=}
local view_timers = {} -- tracked[bufnr] -> uv_timer for the BufEnter delay
local heartbeat_timer = nil
local notified_on_enter = {} -- tracked[bufnr] -> true once we've notified for this visit

-- in_workspace reports whether Neovim's cwd is dir itself or a descendant of
-- it -- used to gate activation when installed permanently (opts.scoped)
-- into a config that loads on every Neovim launch, not just inside the one
-- report-rex workspace this setup() call was generated for.
local function in_workspace(dir)
	local cwd = vim.fn.getcwd()
	return cwd == dir or cwd:sub(1, #dir + 1) == (dir .. "/")
end

-- workspace_relative_path returns bufname relative to Neovim's cwd,
-- normalized to forward slashes -- callers are expected to run `nvim` from
-- the workspace root, the same assumption `rex sync --dir .` makes.
local function workspace_relative_path(bufnr)
	local name = vim.api.nvim_buf_get_name(bufnr)
	if name == "" then
		return nil
	end
	local rel = vim.fn.fnamemodify(name, ":.")
	if vim.fn.fnamemodify(rel, ":p") == rel then
		return nil -- fnamemodify couldn't relativize it (outside cwd): skip
	end
	return (rel:gsub("\\", "/"))
end

local function send_update(bufnr, file, state, dirty)
	tracked[bufnr] = { file = file, state = state, dirty = dirty }
	bridge.send({ op = "update", file = file, state = state, dirty = dirty })
end

local function send_release(bufnr)
	local t = tracked[bufnr]
	if not t then
		return
	end
	bridge.send({ op = "release", file = t.file })
	tracked[bufnr] = nil
	notified_on_enter[bufnr] = nil
end

local function refresh_explorer()
	explorer.refresh(explorer.editing_basenames(remote, cfg.username))
end

local function on_bridge_event(ev)
	if ev.type == "presence" then
		remote[ev.file] = remote[ev.file] or {}
		if ev.action == "idle" then
			remote[ev.file][ev.session_id] = nil
		else
			remote[ev.file][ev.session_id] = { username = ev.username, state = ev.state, dirty = ev.dirty }
		end
		vim.schedule(function()
			vim.cmd("redrawstatus")
			refresh_explorer()
		end)
	elseif ev.type == "status" and not ev.connected and ev.error and cfg and cfg.verbose then
		vim.schedule(function()
			vim.notify("rex-presence: " .. ev.error, vim.log.levels.WARN)
		end)
	end
end

local function maybe_notify_on_enter(bufnr, file)
	if notified_on_enter[bufnr] then
		return
	end
	notified_on_enter[bufnr] = true
	local others = statusline.others_on(remote, file, cfg.username)
	if #others > 0 then
		vim.notify(string.format("⚠ %s is currently editing this file", others[1].username), vim.log.levels.WARN)
	end
end

local function maybe_notify_concurrent_edit(file)
	local is_conflict, who = statusline.is_concurrent_edit(remote, file, cfg.username)
	if is_conflict then
		vim.notify(
			string.format("⚠ CONCURRENT EDIT\n\n%s is currently editing this file.\nYour changes may conflict.", who),
			vim.log.levels.ERROR,
			{ title = "CONCURRENT EDIT" }
		)
	end
end

local function on_buf_enter(bufnr)
	local file = workspace_relative_path(bufnr)
	if not file then
		return
	end
	if view_timers[bufnr] then
		view_timers[bufnr]:stop()
	end
	view_timers[bufnr] = vim.defer_fn(function()
		local already_editing = tracked[bufnr] and tracked[bufnr].state == "editing"
		if vim.api.nvim_get_current_buf() == bufnr and not already_editing then
			send_update(bufnr, file, "viewing", vim.bo[bufnr].modified)
			maybe_notify_on_enter(bufnr, file)
		end
	end, VIEW_DELAY_MS)
end

local function on_insert_enter(bufnr)
	local t = tracked[bufnr]
	local file = (t and t.file) or workspace_relative_path(bufnr)
	if not file then
		return
	end
	send_update(bufnr, file, "editing", true)
	maybe_notify_concurrent_edit(file)
end

local function on_text_changed(bufnr)
	local t = tracked[bufnr]
	if not t then
		return
	end
	send_update(bufnr, t.file, t.state, vim.bo[bufnr].modified)
end

local function start_heartbeat()
	heartbeat_timer = vim.uv.new_timer()
	heartbeat_timer:start(
		HEARTBEAT_MS,
		HEARTBEAT_MS,
		vim.schedule_wrap(function()
			for bufnr, t in pairs(tracked) do
				bridge.send({ op = "update", file = t.file, state = t.state, dirty = t.dirty })
			end
			refresh_explorer()
		end)
	)
end

-- setup(opts): opts.host, opts.username required for presence/sync; opts.port
-- (default 9443), opts.password (default $REX_PASSWORD, then an interactive
-- prompt is NOT available here -- set one of the two for a non-interactive
-- bridge), opts.bin (default "rex" on $PATH), opts.verbose (default false).
-- opts.ca_cert (path to rex-svr's self-signed certificate) or opts.insecure
-- (skip TLS verification) -- exactly one is required, since rex-svr only
-- serves HTTPS/WSS; passed straight through to the spawned rex subprocess.
-- opts.dir: the report-rex workspace's absolute path -- also passed to a
-- background `rex sync` job so the workspace stays synced, not just
-- presence-aware. opts.scoped (default false): when true, setup() does
-- nothing at all (no bridge, no sync job, no autocmds) unless Neovim's cwd
-- is opts.dir or a descendant of it -- for a config installed permanently
-- into a Neovim setup that's also used for unrelated projects.
--
-- opts.lint_enabled (default true) and opts.project_override (default
-- "auto", see lint.lua) configure the linter (issue #26) -- unlike
-- presence/sync, linting works with no host/username at all, so it's wired
-- up before that requirement is checked below.
function M.setup(opts)
	cfg = vim.tbl_extend("force", { port = 9443 }, opts or {})
	if cfg.scoped and cfg.dir and not in_workspace(cfg.dir) then
		return
	end

	local group = vim.api.nvim_create_augroup("RexPresence", { clear = true })

	lint.setup({ enabled = cfg.lint_enabled, override = cfg.project_override })
	vim.api.nvim_create_autocmd("BufWritePost", {
		group = group,
		pattern = "*.md",
		callback = function()
			if cfg.dir then
				lint.run(cfg.dir, cfg.bin)
			end
		end,
	})
	vim.api.nvim_create_user_command("RexLintToggle", function()
		lint.toggle(cfg.dir, cfg.bin)
	end, { desc = "Toggle the `rex report validate` linter on/off" })
	vim.api.nvim_create_user_command("RexLintRun", function()
		if cfg.dir then
			lint.run(cfg.dir, cfg.bin)
		end
	end, { desc = "Run `rex report validate` now and refresh diagnostics" })
	vim.api.nvim_create_user_command("RexProjectOverride", function(args)
		lint.set_override(args.args, cfg.dir, cfg.bin)
	end, {
		nargs = 1,
		complete = function()
			return { "auto", "on", "off" }
		end,
		desc = "Override whether this workspace is treated as a rex report project",
	})
	if cfg.dir then
		lint.run(cfg.dir, cfg.bin)
	end

	if not cfg.host or not cfg.username then
		vim.notify(
			"rex-presence: setup() requires host and username for presence/sync (linting is still available)",
			vim.log.levels.WARN
		)
		return
	end

	vim.api.nvim_set_hl(0, "RexPresenceEditing", { link = "DiagnosticError", default = true })

	bridge.start(cfg, on_bridge_event)
	start_heartbeat()
	if cfg.dir then
		syncjob.start(cfg)
	end

	vim.api.nvim_create_autocmd("BufEnter", {
		group = group,
		callback = function(a)
			on_buf_enter(a.buf)
		end,
	})
	vim.api.nvim_create_autocmd("InsertEnter", {
		group = group,
		callback = function(a)
			on_insert_enter(a.buf)
		end,
	})
	vim.api.nvim_create_autocmd({ "TextChanged", "TextChangedI" }, {
		group = group,
		callback = function(a)
			on_text_changed(a.buf)
		end,
	})
	vim.api.nvim_create_autocmd("BufLeave", {
		group = group,
		callback = function(a)
			if view_timers[a.buf] then
				view_timers[a.buf]:stop()
				view_timers[a.buf] = nil
			end
			send_release(a.buf)
		end,
	})
	vim.api.nvim_create_autocmd({ "BufWinEnter", "FileType" }, {
		group = group,
		callback = function(a)
			if explorer.is_explorer_buf(a.buf) then
				vim.defer_fn(refresh_explorer, 50) -- neo-tree renders asynchronously
			end
		end,
	})
	vim.api.nvim_create_autocmd("VimLeavePre", {
		group = group,
		callback = function()
			for bufnr in pairs(tracked) do
				send_release(bufnr)
			end
			if heartbeat_timer then
				heartbeat_timer:stop()
			end
			bridge.stop()
			syncjob.stop()
		end,
	})
end

-- statusline() returns a short fragment (e.g. "⚠ alice editing") for the
-- current buffer, or "" if no one else is present on it. Wire this into
-- 'statusline'/'winbar'.
function M.statusline()
	if not cfg then
		return ""
	end
	local file = workspace_relative_path(vim.api.nvim_get_current_buf())
	if not file then
		return ""
	end
	return statusline.status_text(remote, file, cfg.username)
end

return M
