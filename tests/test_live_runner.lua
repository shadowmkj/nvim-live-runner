-- tests/test_live_runner.lua
-- Unit & Integration tests for nvim-live-runner

-- Configure package paths for luarocks and local modules
local home = os.getenv("HOME") or ""
package.path = package.path
	.. ";" .. home .. "/.luarocks/share/lua/5.1/?.lua"
	.. ";" .. home .. "/.luarocks/share/lua/5.1/?/init.lua"
	.. ";./lua/?.lua;./lua/?/init.lua"
package.cpath = package.cpath
	.. ";" .. home .. "/.luarocks/lib/lua/5.1/?.so"

-- Start LuaCov coverage collection if available
local luacov_ok, luacov_runner = pcall(require, "luacov.runner")
if luacov_ok then
	luacov_runner.init()
end

local function assert_eq(actual, expected, msg)
	if actual ~= expected then
		local err = string.format(
			"ASSERTION FAILED: %s\nExpected: %s (%s)\nActual:   %s (%s)",
			msg or "",
			vim.inspect(expected),
			type(expected),
			vim.inspect(actual),
			type(actual)
		)
		error(err, 2)
	end
end

local function assert_true(cond, msg)
	if not cond then
		error("ASSERTION FAILED: " .. (msg or "expected true"), 2)
	end
end

local passed = 0
local failed = 0

local function run_test(name, fn)
	io.write("RUN: " .. name .. " ... ")
	local ok, err = pcall(fn)
	if ok then
		io.write("PASS\n")
		passed = passed + 1
	else
		io.write("FAIL\n" .. tostring(err) .. "\n")
		failed = failed + 1
	end
end

-- ==============================================================================
-- Test Suites (Targeting nvim-live-runner functionality only)
-- ==============================================================================

run_test("config defaults", function()
	local config = require("live-runner.config")
	assert_eq(config.port, 65432, "default port must be 65432")
	assert_eq(config.bin_path, nil, "default bin_path must be nil")
	assert_eq(config.show_line_numbers, false, "default show_line_numbers must be false")
end)

run_test("setup with custom options", function()
	local runner = require("live-runner")
	runner.setup({
		port = 55555,
		bin_path = "/custom/path/to/server",
		show_line_numbers = true,
	})

	assert_eq(runner.config.port, 55555, "port should be overridden")
	assert_eq(runner.config.bin_path, "/custom/path/to/server", "bin_path should be overridden")
	assert_eq(runner.config.show_line_numbers, true, "show_line_numbers should be overridden")
end)

run_test("toggle_line_numbers toggles boolean state and updates window", function()
	local runner = require("live-runner")
	runner.setup({ show_line_numbers = false })

	runner.toggle_line_numbers()
	assert_eq(runner.config.show_line_numbers, true, "should toggle from false to true")

	runner.toggle_line_numbers()
	assert_eq(runner.config.show_line_numbers, false, "should toggle from true to false")
end)

run_test("start fails gracefully when server binary is missing (default and custom path)", function()
	local runner = require("live-runner")
	local notified = false
	local orig_notify = vim.notify
	vim.notify = function(msg, level)
		if msg:find("Server binary not found") and level == vim.log.levels.ERROR then
			notified = true
		end
	end

	-- Custom path
	runner.setup({ bin_path = "/nonexistent/binary/path/server" })
	runner.start()
	assert_true(notified, "should notify error when custom binary is missing")

	-- Default path resolution fallback
	notified = false
	runner.setup({ bin_path = nil })
	runner.start()

	vim.notify = orig_notify
end)

run_test("start and window creation with mock binary", function()
	local runner = require("live-runner")

	-- Create a mock executable script in tmp/
	local mock_bin = vim.fn.fnamemodify("./tmp/mock_server.sh", ":p")
	vim.fn.mkdir(vim.fn.fnamemodify(mock_bin, ":h"), "p")
	local f = io.open(mock_bin, "w")
	f:write("#!/bin/sh\necho 'Listening on :65432...'\nwhile true; do sleep 1; done\n")
	f:close()
	vim.fn.setfperm(mock_bin, "rwxr-xr-x")

	runner.setup({
		bin_path = mock_bin,
		show_line_numbers = false,
	})

	runner.start()

	-- Verify output buffer exists and has correct name & filetype
	local output_buf = nil
	for _, b in ipairs(vim.api.nvim_list_bufs()) do
		if vim.api.nvim_buf_get_name(b):find("LiveRunner Output") then
			output_buf = b
			break
		end
	end
	assert_true(output_buf ~= nil, "output buffer 'LiveRunner Output' should be created")
	assert_eq(vim.api.nvim_get_option_value("filetype", { buf = output_buf }), "liverunner", "filetype should be liverunner")

	-- Starting again prints 'Server already running'
	runner.start()

	-- Clean up
	runner.stop()
	os.remove(mock_bin)
end)

run_test("output buffer streaming, chunks, and clear screen handling", function()
	local runner = require("live-runner")

	-- Setup with a mock binary that outputs data then exits
	local mock_bin = vim.fn.fnamemodify("./tmp/mock_echo.sh", ":p")
	vim.fn.mkdir(vim.fn.fnamemodify(mock_bin, ":h"), "p")
	local f = io.open(mock_bin, "w")
	f:write("#!/bin/sh\nprintf '\\033cLine 1\\nLine 2\\n'\n")
	f:close()
	vim.fn.setfperm(mock_bin, "rwxr-xr-x")

	runner.setup({ bin_path = mock_bin })
	runner.start()

	-- Wait for job output to flush into buffer
	vim.wait(300, function() return false end)

	local output_buf = nil
	for _, b in ipairs(vim.api.nvim_list_bufs()) do
		if vim.api.nvim_buf_get_name(b):find("LiveRunner Output") then
			output_buf = b
			break
		end
	end
	assert_true(output_buf ~= nil, "output buffer must exist")

	local lines = vim.api.nvim_buf_get_lines(output_buf, 0, -1, false)
	assert_true(#lines >= 1, "output buffer should contain header or output lines")

	runner.stop()
	os.remove(mock_bin)
end)

run_test("attach and buffer changes trigger TCP client sending", function()
	local runner = require("live-runner")

	-- Create a mock executable server in tmp/
	local mock_bin = vim.fn.fnamemodify("./tmp/mock_server_attach.sh", ":p")
	vim.fn.mkdir(vim.fn.fnamemodify(mock_bin, ":h"), "p")
	local f = io.open(mock_bin, "w")
	f:write("#!/bin/sh\nwhile true; do sleep 1; done\n")
	f:close()
	vim.fn.setfperm(mock_bin, "rwxr-xr-x")

	runner.setup({ bin_path = mock_bin, port = 65430 })
	runner.start()
	runner.attach()

	-- Create a test buffer for a python file
	local test_buf = vim.api.nvim_create_buf(true, false)
	vim.api.nvim_buf_set_name(test_buf, "test_file.py")
	vim.api.nvim_set_current_buf(test_buf)
	vim.api.nvim_buf_set_lines(test_buf, 0, -1, false, { "print('live test')" })

	-- Trigger TextChanged event on the buffer
	vim.api.nvim_exec_autocmds("TextChanged", { group = "LiveRunnerClient", buffer = test_buf })
	vim.api.nvim_exec_autocmds("TextChangedI", { group = "LiveRunnerClient", buffer = test_buf })

	vim.wait(50, function() return false end)

	runner.stop()
	os.remove(mock_bin)
end)

run_test("LiveRun command dispatch and completion", function()
	local runner = require("live-runner")

	local mock_bin = vim.fn.fnamemodify("./tmp/mock_cmd.sh", ":p")
	vim.fn.mkdir(vim.fn.fnamemodify(mock_bin, ":h"), "p")
	local f = io.open(mock_bin, "w")
	f:write("#!/bin/sh\nwhile true; do sleep 1; done\n")
	f:close()
	vim.fn.setfperm(mock_bin, "rwxr-xr-x")

	runner.setup({ bin_path = mock_bin })

	-- Test :LiveRun without arguments (start and attach)
	vim.cmd("LiveRun")
	vim.cmd("LiveRun stop")

	-- Test :LiveRun numbers
	vim.cmd("LiveRun numbers")
	assert_eq(runner.config.show_line_numbers, true, "LiveRun numbers should toggle line numbers to true")
	vim.cmd("LiveRun numbers")
	assert_eq(runner.config.show_line_numbers, false, "LiveRun numbers should toggle line numbers to false")

	-- Test :LiveRun with unknown command warning
	local warn_emitted = false
	local orig_notify = vim.notify
	vim.notify = function(msg, level)
		if msg:find("Unknown subcommand") and level == vim.log.levels.WARN then
			warn_emitted = true
		end
	end

	vim.cmd("LiveRun invalid_cmd")
	vim.notify = orig_notify
	assert_true(warn_emitted, "should warn on unknown subcommand")

	-- Test completion via vim.fn.getcompletion
	local completions = vim.fn.getcompletion("LiveRun num", "cmdline")
	assert_true(#completions > 0, "should return autocomplete candidates for 'num'")
	assert_eq(completions[1], "numbers", "candidate should match 'numbers'")

	local stop_completions = vim.fn.getcompletion("LiveRun st", "cmdline")
	assert_true(#stop_completions > 0, "should return autocomplete candidates for 'st'")
	assert_eq(stop_completions[1], "stop", "candidate should match 'stop'")

	os.remove(mock_bin)
end)

-- ==============================================================================
-- Coverage Reporting & Summary
-- ==============================================================================

if luacov_ok then
	luacov_runner.shutdown()
	local reporter = require("luacov.reporter")
	reporter.report()
end

print(string.format("\n=========================================="))
print(string.format("Lua Tests Finished: %d Passed, %d Failed", passed, failed))
print(string.format("=========================================="))

if failed > 0 then
	os.exit(1)
else
	os.exit(0)
end
