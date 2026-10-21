-- Spawns `rex sync` as a background job so opening Neovim in a report-rex
-- workspace keeps it collaboratively in sync, not just presence-aware.
-- Mirrors bridge.lua's jobstart pattern, but rex sync's stdout is
-- human-readable log lines (not NDJSON), so there's no parser here -- just
-- optional vim.notify passthrough.
local M = {}

M.job_id = nil

local function notify_lines(data, level)
	for _, line in ipairs(data or {}) do
		if line ~= "" then
			vim.schedule(function()
				vim.notify("rex-sync: " .. line, level)
			end)
		end
	end
end

-- start spawns `<bin> sync --host ... --username ... --dir ...`. Output is
-- only surfaced (via vim.notify) when opts.verbose is set -- quiet by
-- default, matching bridge.lua.
function M.start(opts)
	local cmd = {
		opts.bin or "rex",
		"sync",
		"--host",
		opts.host,
		"--port",
		tostring(opts.port or 9443),
		"--username",
		opts.username,
		"--dir",
		opts.dir,
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
	M.job_id = vim.fn.jobstart(cmd, {
		env = env,
		on_stdout = function(_, data)
			if opts.verbose then
				notify_lines(data, vim.log.levels.INFO)
			end
		end,
		on_stderr = function(_, data)
			if opts.verbose then
				notify_lines(data, vim.log.levels.WARN)
			end
		end,
		on_exit = function()
			M.job_id = nil
		end,
	})
	if M.job_id <= 0 then
		M.job_id = nil
	end
end

function M.stop()
	if M.job_id then
		vim.fn.jobwait({ M.job_id }, 300)
		vim.fn.jobstop(M.job_id)
		M.job_id = nil
	end
end

return M
