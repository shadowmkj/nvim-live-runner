-- tests/test_live_runner.lua
-- Unit & Integration tests for nvim-live-runner using LuaUnit testing framework

local home = os.getenv("HOME") or ""
package.path = package.path
	.. ";" .. home .. "/.luarocks/share/lua/5.1/?.lua"
	.. ";" .. home .. "/.luarocks/share/lua/5.1/?/init.lua"
	.. ";./lua/?.lua;./lua/?/init.lua"
package.cpath = package.cpath
	.. ";" .. home .. "/.luarocks/lib/lua/5.1/?.so"

-- Initialize LuaCov coverage profiling
local luacov_ok, luacov_runner = pcall(require, "luacov.runner")
if luacov_ok then
	luacov_runner.init()
end

local lu = require("luaunit")

-- Helper function to safely create mock executable scripts
local function create_mock_script(rel_path, content)
	local full_path = vim.fn.fnamemodify(rel_path, ":p")
	local dir = vim.fn.fnamemodify(full_path, ":h")
	vim.fn.mkdir(dir, "p")
	local f, err = io.open(full_path, "w")
	if not f then
		error("Failed to open " .. full_path .. " for writing: " .. tostring(err))
	end
	f:write(content or "#!/bin/sh\nwhile true; do sleep 1; done\n")
	f:close()
	vim.fn.setfperm(full_path, "rwxr-xr-x")
	return full_path
end

-- ==============================================================================
-- Test Suite: LiveRunnerConfiguration
-- ==============================================================================

TestLiveRunnerConfiguration = {}

function TestLiveRunnerConfiguration:testDefaultConfiguration()
	local config = require("live-runner.config")
	lu.assertEquals(config.port, 65432, "default port should be 65432")
	lu.assertNil(config.bin_path, "default bin_path should be nil")
	lu.assertFalse(config.show_line_numbers, "default show_line_numbers should be false")
end

function TestLiveRunnerConfiguration:testSetupMergesCustomOptions()
	local runner = require("live-runner")
	runner.setup({
		port = 55555,
		bin_path = "/custom/path/to/server",
		show_line_numbers = true,
	})

	lu.assertEquals(runner.config.port, 55555, "port should be overridden")
	lu.assertEquals(runner.config.bin_path, "/custom/path/to/server", "bin_path should be overridden")
	lu.assertTrue(runner.config.show_line_numbers, "show_line_numbers should be overridden")
end

-- ==============================================================================
-- Test Suite: LiveRunnerWindowAndNumbers
-- ==============================================================================

TestLiveRunnerWindowAndNumbers = {}

function TestLiveRunnerWindowAndNumbers:testToggleLineNumbersState()
	local runner = require("live-runner")
	runner.setup({ show_line_numbers = false })

	runner.toggle_line_numbers()
	lu.assertTrue(runner.config.show_line_numbers, "toggling from false should yield true")

	runner.toggle_line_numbers()
	lu.assertFalse(runner.config.show_line_numbers, "toggling from true should yield false")
end

-- ==============================================================================
-- Test Suite: LiveRunnerLifecycle
-- ==============================================================================

TestLiveRunnerLifecycle = {}

function TestLiveRunnerLifecycle:setUp()
	self.mock_bin = create_mock_script("./tmp/mock_server_unit.sh", "#!/bin/sh\necho 'Listening on :65432...'\nwhile true; do sleep 1; done\n")
end

function TestLiveRunnerLifecycle:tearDown()
	local runner = require("live-runner")
	runner.stop()
	if vim.fn.filereadable(self.mock_bin) == 1 then
		os.remove(self.mock_bin)
	end
end

function TestLiveRunnerLifecycle:testMissingBinaryNotification()
	local runner = require("live-runner")
	local notified = false
	local orig_notify = vim.notify
	vim.notify = function(msg, level)
		if msg:find("Server binary not found") and level == vim.log.levels.ERROR then
			notified = true
		end
	end

	-- Test custom missing binary path
	runner.setup({ bin_path = "/nonexistent/binary/path/server" })
	runner.start()
	lu.assertTrue(notified, "should notify when custom binary is missing")

	-- Test default binary resolution fallback when binary is missing
	notified = false
	runner.setup({ bin_path = nil })
	runner.start()

	vim.notify = orig_notify
end

function TestLiveRunnerLifecycle:testStartAndWindowCreation()
	local runner = require("live-runner")
	runner.setup({
		bin_path = self.mock_bin,
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
	lu.assertNotNil(output_buf, "output buffer 'LiveRunner Output' should be created")
	lu.assertEquals(vim.api.nvim_get_option_value("filetype", { buf = output_buf }), "liverunner", "filetype should be liverunner")

	-- Re-invoking start when already running should be idempotent
	runner.start()

	-- Clean up via stop()
	runner.stop()
end

function TestLiveRunnerLifecycle:testOutputStreamingAndClearScreen()
	local runner = require("live-runner")
	local echo_bin = create_mock_script("./tmp/mock_echo_unit.sh", "#!/bin/sh\nprintf '\\033cLine 1\\nLine 2\\n'\n")

	runner.setup({ bin_path = echo_bin })
	runner.start()

	vim.wait(300, function() return false end)

	local output_buf = nil
	for _, b in ipairs(vim.api.nvim_list_bufs()) do
		if vim.api.nvim_buf_get_name(b):find("LiveRunner Output") then
			output_buf = b
			break
		end
	end
	lu.assertNotNil(output_buf, "output buffer must exist")

	local lines = vim.api.nvim_buf_get_lines(output_buf, 0, -1, false)
	lu.assertTrue(#lines >= 1, "output buffer should contain lines")

	runner.stop()
	os.remove(echo_bin)
end

-- ==============================================================================
-- Test Suite: LiveRunnerClientAndCommands
-- ==============================================================================

TestLiveRunnerClientAndCommands = {}

function TestLiveRunnerClientAndCommands:testAttachAndBufferEvents()
	local runner = require("live-runner")
	local mock_bin = create_mock_script("./tmp/mock_attach_unit.sh", "#!/bin/sh\nwhile true; do sleep 1; done\n")

	runner.setup({ bin_path = mock_bin, port = 65431 })
	runner.start()
	runner.attach()

	-- Check autocommands exist
	local autocmds = vim.api.nvim_get_autocmds({ group = "LiveRunnerClient" })
	lu.assertTrue(#autocmds > 0, "autocommands should be created in LiveRunnerClient group")

	-- Create test buffer and trigger autocommands
	local test_buf = vim.api.nvim_create_buf(true, false)
	vim.api.nvim_buf_set_name(test_buf, "test_script.py")
	vim.api.nvim_set_current_buf(test_buf)
	vim.api.nvim_buf_set_lines(test_buf, 0, -1, false, { "print('unit test')" })

	vim.api.nvim_exec_autocmds("TextChanged", { group = "LiveRunnerClient", buffer = test_buf })
	vim.api.nvim_exec_autocmds("TextChangedI", { group = "LiveRunnerClient", buffer = test_buf })

	vim.wait(50, function() return false end)

	runner.stop()
	os.remove(mock_bin)
end

function TestLiveRunnerClientAndCommands:testUserCommandExecutionAndCompletion()
	local runner = require("live-runner")
	local mock_bin = create_mock_script("./tmp/mock_cmd_unit.sh", "#!/bin/sh\nwhile true; do sleep 1; done\n")

	runner.setup({ bin_path = mock_bin })

	-- Test :LiveRun without arguments
	vim.cmd("LiveRun")
	vim.cmd("LiveRun stop")

	-- Test :LiveRun numbers
	vim.cmd("LiveRun numbers")
	lu.assertTrue(runner.config.show_line_numbers, "LiveRun numbers should toggle show_line_numbers to true")
	vim.cmd("LiveRun numbers")
	lu.assertFalse(runner.config.show_line_numbers, "LiveRun numbers should toggle show_line_numbers back to false")

	-- Test unknown subcommand warning
	local warn_emitted = false
	local orig_notify = vim.notify
	vim.notify = function(msg, level)
		if msg:find("Unknown subcommand") and level == vim.log.levels.WARN then
			warn_emitted = true
		end
	end

	vim.cmd("LiveRun unknown_option")
	vim.notify = orig_notify
	lu.assertTrue(warn_emitted, "should warn on unknown subcommand")

	-- Test completion candidates
	local num_candidates = vim.fn.getcompletion("LiveRun num", "cmdline")
	lu.assertTrue(#num_candidates > 0, "should return completion for 'num'")
	lu.assertEquals(num_candidates[1], "numbers", "candidate should be 'numbers'")

	local stop_candidates = vim.fn.getcompletion("LiveRun st", "cmdline")
	lu.assertTrue(#stop_candidates > 0, "should return completion for 'st'")
	lu.assertEquals(stop_candidates[1], "stop", "candidate should be 'stop'")

	os.remove(mock_bin)
end

-- ==============================================================================
-- Execute LuaUnit Test Runner
-- ==============================================================================

local exit_code = lu.LuaUnit.run()

if luacov_ok then
	luacov_runner.shutdown()
	local reporter = require("luacov.reporter")
	reporter.report()
end

os.exit(exit_code)
