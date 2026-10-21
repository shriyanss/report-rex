-- Manual self-check for explorer.lua's pure logic (editing_basenames,
-- find_matches). Not wired into CI -- there's no Neovim in the CI image --
-- run it locally after touching explorer.lua:
--
--   nvim --headless -l cli/editors/nvim/lua/rex-presence/explorer_spec.lua
--
-- Exits 0 and prints "ok" on success, non-zero with a message on failure.
package.path = vim.fn.getcwd() .. "/lua/?.lua;" .. package.path

local explorer = require("rex-presence.explorer")

local function check(name, cond)
	if not cond then
		io.stderr:write("FAIL: " .. name .. "\n")
		os.exit(1)
	end
end

-- editing_basenames
do
	local remote = {
		["reports/vuln-1.md"] = { s1 = { username = "alice", state = "editing", dirty = true } },
		["reports/vuln-2.md"] = { s2 = { username = "bob", state = "viewing", dirty = false } },
		["reports/vuln-3.md"] = { s3 = { username = "carol", state = "editing", dirty = false } },
	}
	local names = explorer.editing_basenames(remote, "bob")
	check("editing_basenames includes another user's editing file", names["vuln-1.md"] == true)
	check("editing_basenames excludes a merely-viewed file", names["vuln-2.md"] == nil)
	check("editing_basenames includes a third user's editing file", names["vuln-3.md"] == true)

	local own = explorer.editing_basenames(remote, "alice")
	check("editing_basenames excludes the caller's own session", own["vuln-1.md"] == nil)
end

-- find_matches
do
	local basenames = { ["vuln-1.md"] = true }
	local matches = explorer.find_matches("  vuln-1.md", basenames)
	check("find_matches finds one match", #matches == 1)
	check("find_matches span covers the basename", matches[1][1] == 2 and matches[1][2] == 11)

	check("find_matches finds nothing for an unrelated line", #explorer.find_matches("  vuln-2.md", basenames) == 0)
end

print("ok")
