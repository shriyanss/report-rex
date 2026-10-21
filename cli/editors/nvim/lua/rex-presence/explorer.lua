-- Highlights files someone else is currently editing inside a file-explorer
-- buffer (netrw, neo-tree, nvim-tree). There's no per-explorer integration
-- here -- rather than owning a component in the user's own neo-tree config,
-- this scans the explorer's *rendered text* for a matching basename and
-- highlights it, which works generically across explorers (and is what's
-- reachable on a netrw-only box with no plugin manager).
--
-- ponytail: matching is by basename only, so two files with the same name in
-- different directories both highlight. Upgrade to full relative-path-aware
-- matching (walking each explorer's own tree/node API) if that ever bites.
local M = {}

local ns = vim.api.nvim_create_namespace("rex_presence_explorer")

local EXPLORER_FILETYPES = {
	netrw = true,
	["neo-tree"] = true,
	NvimTree = true,
	snacks_picker_list = true, -- snacks.nvim's explorer (LazyVim's newer default)
}

function M.is_explorer_buf(bufnr)
	return EXPLORER_FILETYPES[vim.bo[bufnr].filetype] == true
end

-- editing_basenames returns the basename of every file (other than
-- own_username's own sessions) currently in the "editing" state, as a set.
-- remote[file][session_id] = {username=, state=, dirty=}
function M.editing_basenames(remote, own_username)
	local out = {}
	for file, sessions in pairs(remote) do
		for _, entry in pairs(sessions) do
			if entry.username ~= own_username and entry.state == "editing" then
				out[file:match("([^/]+)$") or file] = true
			end
		end
	end
	return out
end

-- find_matches returns a list of {s, e} 0-indexed byte ranges in `line`
-- covering every basename in `basenames` that appears in it.
function M.find_matches(line, basenames)
	local matches = {}
	for basename in pairs(basenames) do
		local s, e = line:find(basename, 1, true)
		if s then
			table.insert(matches, { s - 1, e })
		end
	end
	return matches
end

-- refresh re-scans every loaded explorer buffer and re-applies highlights
-- for the given editing_basenames set (as returned by M.editing_basenames).
function M.refresh(editing_basenames)
	if next(editing_basenames) == nil then
		for _, bufnr in ipairs(vim.api.nvim_list_bufs()) do
			if vim.api.nvim_buf_is_loaded(bufnr) then
				vim.api.nvim_buf_clear_namespace(bufnr, ns, 0, -1)
			end
		end
		return
	end
	for _, bufnr in ipairs(vim.api.nvim_list_bufs()) do
		if vim.api.nvim_buf_is_loaded(bufnr) and M.is_explorer_buf(bufnr) then
			vim.api.nvim_buf_clear_namespace(bufnr, ns, 0, -1)
			local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
			for lnum, line in ipairs(lines) do
				for _, span in ipairs(M.find_matches(line, editing_basenames)) do
					vim.api.nvim_buf_add_highlight(bufnr, ns, "RexPresenceEditing", lnum - 1, span[1], span[2])
				end
			end
		end
	end
end

return M
