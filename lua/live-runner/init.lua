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
	vim.cmd("vsplit")
	output_win = vim.api.nvim_get_current_win()
	vim.api.nvim_win_set_buf(output_win, output_buf)
	vim.cmd("wincmd p")
end

function M.start()
	local bin = M.config.bin_path or get_binary_path()
	if server_job_id then
		print("Server already running")
		return
	end
	ensure_output_window()
	server_job_id = vim.fn.jobstart({ bin }, {
		stdout_buffered = false,
		on_stdout = function(_, data)
			if data then
				vim.api.nvim_buf_set_lines(output_buf, -1, -1, false, data)
			end
		end,
		on_stderr = function(_, data)
			if data then
				vim.api.nvim_buf_set_lines(output_buf, -1, -1, false, data)
			end
		end,
	})
end

function M.attach()
	local group = vim.api.nvim_create_augroup("LiveRunnerClient", { clear = true })
	local function send_buffer_to_tcp()
		if not server_job_id then
			return
		end

		local lines = vim.api.nvim_buf_get_lines(0, 0, -1, false)
		local data = table.concat(lines, "\n")
		local client = uv.new_tcp()
		client:connect("127.0.0.1", M.config.port, function(err)
			if err then
				client:close()
				return
			end
			client:write(data, function()
				client:shutdown()
				client:close()
			end)
		end)
	end

	vim.api.nvim_create_autocmd({ "TextChanged", "TextChangedI" }, {
		group = group,
		pattern = "*.py",
		callback = function()
			send_buffer_to_tcp()
		end,
	})
end

function M.setup(opts)
	M.config = vim.tbl_deep_extend("force", M.config, opts or {})
	vim.api.nvim_create_user_command("LiveRun", function()
		M.start()
		M.attach()
	end, {})
end

return M
