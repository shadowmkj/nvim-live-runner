local M = {}
local uv = vim.loop

M.config = {
	port = 65432,
	bin_path = nil,
}

local server_job_id = nil
local output_buf = nil
local output_win = nil

local function get_binary_path()
	local plugin_root = debug.getinfo(1).source:sub(2):match("(.*/)")
	return plugin_root .. "../../src/server"
end

local function ensure_output_window()
	if output_buf and vim.api.nvim_buf_is_valid(output_buf) then
		if output_win and vim.api.nvim_win_is_valid(output_win) then
			return
		end
	else
		output_buf = vim.api.nvim_create_buf(false, true)
		vim.api.nvim_buf_set_name(output_buf, "LiveRunner Output")
		vim.api.nvim_set_option_value("filetype", "liverunner", { buf = output_buf })
	end
	vim.cmd("vs")
	vim.cmd("wincmd p")
	output_win = vim.api.nvim_get_current_win()
	vim.api.nvim_win_set_buf(output_win, output_buf)
	vim.cmd("wincmd p")
end

local function update_output_buffer(data)
	if not data or #data == 0 then
		return
	end
	if not output_buf or not vim.api.nvim_buf_is_valid(output_buf) then
		return
	end

	-- Check for ANSI clear screen escape sequence (\033c or \27c)
	local first_line = data[1] or ""
	if first_line:find("\033c") or first_line:find("\27c") then
		data[1] = first_line:gsub("\033c", ""):gsub("\27c", "")
		vim.api.nvim_buf_set_lines(output_buf, 0, -1, false, { "-----OUTPUT-----" })
	end

	-- Ignore empty trailing newline chunk from jobstart
	if #data == 1 and data[1] == "" then
		return
	end

	local count = vim.api.nvim_buf_line_count(output_buf)
	vim.api.nvim_buf_set_lines(output_buf, count, count, false, data)
end

function M.start()
	local bin = M.config.bin_path or get_binary_path()
	if vim.fn.filereadable(bin) == 0 and vim.fn.executable(bin) == 0 then
		local err_msg = "LiveRunner: Server binary not found at '" .. bin .. "'. Please build it via 'cd src && go build -o server'."
		vim.notify(err_msg, vim.log.levels.ERROR)
		return
	end

	ensure_output_window()
	if server_job_id then
		print("Server already running")
		return
	end

	local port = tostring(M.config.port or 65432)
	server_job_id = vim.fn.jobstart({ bin, "-port", port }, {
		stdout_buffered = false,
		on_stdout = function(_, data)
			update_output_buffer(data)
		end,
		on_stderr = function(_, data)
			update_output_buffer(data)
		end,
	})
	vim.api.nvim_buf_set_lines(output_buf, 0, -1, false, { "-----OUTPUT-----" })
end

function M.attach()
	local group = vim.api.nvim_create_augroup("LiveRunnerClient", { clear = true })
	local function send_buffer_to_tcp()
		if not server_job_id then
			return
		end

		local active_buf = vim.api.nvim_get_current_buf()
		local buf_name = vim.api.nvim_buf_get_name(active_buf)
		local buffer_extension = buf_name:match("^.+(%..+)$") or ""

		local lines = vim.api.nvim_buf_get_lines(active_buf, 0, -1, false)
		local code = table.concat(lines, "\n")
		-- Protocol: send file extension header line followed by source code
		local payload = buffer_extension .. "\n" .. code

		local client = uv.new_tcp()
		client:connect("127.0.0.1", M.config.port, function(err)
			if err then
				client:close()
				return
			end
			client:write(payload, function()
				client:shutdown()
				client:close()
			end)
		end)
	end

	vim.api.nvim_create_autocmd({ "TextChanged", "TextChangedI" }, {
		group = group,
		pattern = { "*.py", "*.go", "*.lua", "*.js" },
		callback = function()
			send_buffer_to_tcp()
		end,
	})
end

function M.setup(opts)
	M.config = vim.tbl_deep_extend("force", M.config, opts or {})
	vim.api.nvim_create_user_command("LiveRun", function(opts)
		if #opts.args == 0 then
			M.start()
			M.attach()
		else
			if server_job_id then
				vim.fn.jobstop(server_job_id)
				server_job_id = nil
			end
			if output_win and vim.api.nvim_win_is_valid(output_win) then
				vim.api.nvim_win_close(output_win, true)
				output_win = nil
			end
		end
	end, { nargs = "?" })
end

return M
