-- Renders remote presence state for the statusline/winbar and decides when
-- a notification is warranted. Pure functions over the caller's remote-state
-- table, so they're trivially testable without a running job or buffers.
local M = {}

-- remote[file][session_id] = {username=, state=, dirty=}

-- others_on returns every remote entry on `file` excluding `own_username`
-- (the server broadcasts every session's updates to every connection,
-- including the sender's own).
function M.others_on(remote, file, own_username)
	local out = {}
	local by_file = remote[file]
	if not by_file then
		return out
	end
	for _, entry in pairs(by_file) do
		if entry.username ~= own_username then
			table.insert(out, entry)
		end
	end
	return out
end

-- status_text returns a short statusline fragment for `file`, or "" if no
-- one else is present on it. Editing takes precedence over viewing when
-- multiple sessions are present.
function M.status_text(remote, file, own_username)
	local others = M.others_on(remote, file, own_username)
	if #others == 0 then
		return ""
	end
	local editing, viewing = nil, nil
	for _, e in ipairs(others) do
		if e.state == "editing" and not editing then
			editing = e
		elseif not viewing then
			viewing = e
		end
	end
	local who = editing or viewing
	local verb = editing and "editing" or "viewing"
	return "⚠ " .. who.username .. " " .. verb
end

-- is_concurrent_edit reports whether `file` already has another session in
-- the "editing" state -- the trigger for the stronger warning.
function M.is_concurrent_edit(remote, file, own_username)
	for _, e in ipairs(M.others_on(remote, file, own_username)) do
		if e.state == "editing" then
			return true, e.username
		end
	end
	return false, nil
end

return M
