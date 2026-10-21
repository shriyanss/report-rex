-- Wraps `rex presence` as a background job, speaking newline-delimited JSON
-- over stdin/stdout. This is the only file that touches vim.fn.jobstart, so
-- the line-parsing logic (parse_line) stays a pure function other code --
-- and bridge_spec.lua -- can exercise without spawning a real process.
local M = {}

M.job_id = nil
local pending_out = ""

-- parse_line decodes one newline-delimited JSON event from `rex presence`.
-- Returns nil for a blank or malformed line -- presence is advisory, so a
-- corrupt line is dropped rather than raised as an error.
function M.parse_line(line)
	if line == nil or line == "" then
		return nil
	end
	local ok, decoded = pcall(vim.json.decode, line)
	if not ok or type(decoded) ~= "table" then
		return nil
	end
	return decoded
end

-- feed_stdout processes one on_stdout callback's data (a list of lines,
-- where jobstart may split a line across calls -- the last entry is either
-- "" if the previous line ended cleanly, or a partial line continued on the
-- next call). Returns the decoded events found and the still-pending
-- partial line, so callers (and tests) don't need job_id/handle_line state.
function M.feed_stdout(pending, data, on_event)
	if not data then
		return pending
	end
	for i, chunk in ipairs(data) do
		if i < #data then
			local ev = M.parse_line(pending .. chunk)
			if ev then
				on_event(ev)
			end
			pending = ""
		else
			pending = pending .. chunk
		end
	end
	return pending
end

-- start spawns `<bin> presence --host ... --username ...`. on_event(tbl) is
-- called for every decoded stdout line (both "presence" and "status"
-- events -- callers dispatch on .type).
function M.start(opts, on_event)
	local cmd = {
		opts.bin or "rex",
		"presence",
		"--host",
		opts.host,
		"--port",
		tostring(opts.port or 9443),
		"--username",
		opts.username,
	}
	if opts.insecure then
		table.insert(cmd, "--insecure")
	elseif opts.ca_cert then
		table.insert(cmd, "--ca-cert")
		table.insert(cmd, opts.ca_cert)
	end
	local env = nil
	if opts.password then
		env = { REX_PASSWORD = opts.password }
	end
	pending_out = ""
	M.job_id = vim.fn.jobstart(cmd, {
		env = env,
		on_stdout = function(_, data)
			pending_out = M.feed_stdout(pending_out, data, on_event)
		end,
		on_stderr = function() end, -- rex presence logs its own errors as status events on stdout
		on_exit = function()
			M.job_id = nil
		end,
	})
	if M.job_id <= 0 then
		M.job_id = nil
	end
end

function M.send(tbl)
	if not M.job_id then
		return -- bridge not running: fail open, editing must never block on this
	end
	vim.fn.chansend(M.job_id, vim.json.encode(tbl) .. "\n")
end

function M.stop()
	if M.job_id then
		-- Give a just-queued chansend (e.g. a release on VimLeavePre) a chance
		-- to actually reach the child's stdin before killing it -- jobstop
		-- right after chansend can win the race and silently drop it.
		vim.fn.jobwait({ M.job_id }, 300)
		vim.fn.jobstop(M.job_id)
		M.job_id = nil
	end
end

return M
