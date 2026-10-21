-- Manual self-check for the pure parsing/statusline logic (no jobstart, no
-- real `rex presence` process). Not wired into CI -- there's no Neovim in
-- the CI image -- run it locally after touching bridge.lua/statusline.lua:
--
--   nvim --headless -l cli/editors/nvim/lua/rex-presence/bridge_spec.lua
--
-- Exits 0 and prints "ok" on success, non-zero with a message on failure.
package.path = vim.fn.getcwd() .. "/lua/?.lua;" .. package.path

local bridge = require("rex-presence.bridge")
local statusline = require("rex-presence.statusline")

local function check(name, cond)
	if not cond then
		io.stderr:write("FAIL: " .. name .. "\n")
		os.exit(1)
	end
end

-- parse_line
check("parse_line decodes valid JSON", (function()
	local ev = bridge.parse_line('{"type":"status","connected":true}')
	return ev ~= nil and ev.type == "status" and ev.connected == true
end)())

check("parse_line drops malformed JSON", bridge.parse_line("not json") == nil)
check("parse_line drops empty/nil lines", bridge.parse_line("") == nil and bridge.parse_line(nil) == nil)

-- feed_stdout: single complete line
do
	local seen = {}
	local pending = bridge.feed_stdout("", { '{"type":"status","connected":true}', "" }, function(ev)
		table.insert(seen, ev)
	end)
	check("feed_stdout parses one complete line", #seen == 1 and seen[1].type == "status")
	check("feed_stdout has no pending remainder after a clean line", pending == "")
end

-- feed_stdout: a line split across two on_stdout calls
do
	local seen = {}
	local pending = bridge.feed_stdout("", { '{"type":"pres' }, function(ev)
		table.insert(seen, ev)
	end)
	check("feed_stdout buffers a partial line", pending == '{"type":"pres' and #seen == 0)
	pending = bridge.feed_stdout(pending, { 'ence","file":"a.md"}', "" }, function(ev)
		table.insert(seen, ev)
	end)
	check("feed_stdout completes a line split across calls", #seen == 1 and seen[1].file == "a.md")
end

-- statusline
do
	local remote = { ["a.md"] = { s1 = { username = "alice", state = "editing", dirty = true } } }
	check("others_on excludes own username", #statusline.others_on(remote, "a.md", "alice") == 0)
	check("others_on includes other users", #statusline.others_on(remote, "a.md", "bob") == 1)
	check("status_text reports editing", statusline.status_text(remote, "a.md", "bob") == "⚠ alice editing")
	check("status_text empty when no one else present", statusline.status_text(remote, "b.md", "bob") == "")
	local conflict, who = statusline.is_concurrent_edit(remote, "a.md", "bob")
	check("is_concurrent_edit true when someone else is editing", conflict and who == "alice")
	local no_conflict = statusline.is_concurrent_edit(remote, "a.md", "alice")
	check("is_concurrent_edit false for the editor's own session", not no_conflict)
end

print("ok")
