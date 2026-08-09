# nvim-live-runner 🚀

[![Neovim](https://img.shields.io/badge/Neovim-0.7+-57A143.svg?style=for-the-badge&logo=neovim&logoColor=white)](https://neovim.io)
[![Go Version](https://img.shields.io/badge/Go-1.18+-00ADD8.svg?style=for-the-badge&logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=for-the-badge)](https://opensource.org/licenses/MIT)

A simple, real-time code runner for Neovim that provides instant feedback as you write code. Type in your Neovim buffer and see execution results update live in a split output window!

---

## ⚡ Quick Navigation

| 🚀 [Quick Start](#-usage) | 📦 [Installation](#-installation) | ⚙️ [Configuration](#-configuration) | 🐛 [Troubleshooting](#-troubleshooting--faq) |
|---|---|---|---|

---

## 📚 Table of Contents

- [✨ Features](#-features)
- [🏗️ Architecture & How It Works](#️-architecture--how-it-works)
- [🌍 Supported Languages](#-supported-languages)
- [✅ Requirements](#-requirements)
- [📦 Installation](#-installation)
  - [lazy.nvim](#lazynvim)
  - [packer.nvim](#packernvim)
  - [vim-plug](#vim-plug)
  - [Manual Compilation](#manual-compilation)
- [🚀 Usage](#-usage)
  - [Commands](#commands)
  - [Keymap Example](#keymap-example)
- [⚙️ Configuration](#-configuration)
  - [Configuration Options](#configuration-options)
  - [Environment Variables](#environment-variables)
- [🐛 Troubleshooting & FAQ](#-troubleshooting--faq)
- [🛣️ Roadmap](#-roadmap)
- [❤️ Contributing](#️-contributing)
- [📄 License](#-license)

---

## ✨ Features

- **⚡ Instant Live Feedback**: See code execution output update dynamically as you type (`TextChanged` and `TextChangedI`).
- **🚀 High Performance Backend**: Powered by a lightweight Go TCP server for minimal CPU usage and fast execution.
- **🔄 Dynamic Language Detection**: Seamlessly switch between Python, Go, Lua, and JavaScript buffers without restarting the backend process.
- **⏱️ Smart Debouncing**: Built-in 250ms debouncer prevents CPU thrashing during rapid keystrokes.
- **🛡️ Process Timeout Protection**: Automatically terminates runaway scripts or infinite loops to keep Neovim responsive.
- **📦 Zero Heavy Dependencies**: Simple Lua frontend paired with a standalone compiled Go binary.

---

## 🏗️ Architecture & How It Works

```
 ┌──────────────────┐           TCP Socket           ┌─────────────────────┐
 │  Neovim Editor   │ ─────────────────────────────> │  Go Backend Server  │
 │ (Buffer Changes) │     Payload: .py\n<code>        │  (src/server :port) │
 └────────┬─────────┘                                └──────────┬──────────┘
          │                                                     │
          │  Displays Output                                    │ Executes via
          ▼                                                     ▼
 ┌──────────────────┐                                ┌─────────────────────┐
 │ Live Output Split│ <───────────────────────────── │ Language Runtime    │
 │ (Scratch Buffer) │        stdout / stderr         │ (python, node, etc) │
 └──────────────────┘                                └─────────────────────┘
```

1. **Buffer Event**: When text changes in an active supported buffer, Neovim triggers an autocommand.
2. **TCP Stream**: The Lua client sends the file extension header and full buffer contents over TCP to `127.0.0.1:<port>`.
3. **Debounced Execution**: The Go server debounces incoming payloads, executes the code using the matching runtime, and captures output.
4. **Live Stream**: Terminal output is streamed back to Neovim's `LiveRunner Output` split window in real time.

---

## 🌍 Supported Languages

| Language | Extension | Default Runtime Command |
|---|---|---|
| **Python** | `.py` | `python3` |
| **Go** | `.go` | `go run` |
| **Lua** | `.lua` | `lua` |
| **JavaScript** | `.js` | `node` |

---

## ✅ Requirements

Before installing, ensure the following dependencies are available on your system path:

- **Neovim**: `>= 0.7.0`
- **Go**: `>= 1.18` (Required to compile and run the backend server)
- **Language Runtimes**:
  - Python: `python3`
  - Node.js: `node` (v16+)
  - Lua: `lua` (v5.1+)

---

## 📦 Installation

### [lazy.nvim](https://github.com/folke/lazy.nvim)

```lua
return {
    "shadowmkj/nvim-live-runner",
    build = "cd src && go build -o server", -- Compiles backend server on install/update
    opts = {
        port = 65432,
    },
    config = function(_, opts)
        require("live-runner").setup(opts)
    end,
}
```

### [packer.nvim](https://github.com/wbthomason/packer.nvim)

```lua
use {
    "shadowmkj/nvim-live-runner",
    run = "cd src && go build -o server",
    config = function()
        require("live-runner").setup({
            port = 65432,
        })
    end,
}
```

### [vim-plug](https://github.com/junegunn/vim-plug)

```vim
Plug 'shadowmkj/nvim-live-runner', { 'do': 'cd src && go build -o server' }
```

### Manual Compilation

If you prefer to compile the backend server binary manually:

```bash
cd ~/.local/share/nvim/plugged/nvim-live-runner/src
go build -o server
```

---

## 🚀 Usage

### Commands

| Command | Description |
|---|---|
| `:LiveRun` | Starts the live runner server and opens the split output window. |
| `:LiveRun stop` | Stops the live runner background process and closes the output split window. |
| `:LiveRun toggle-numbers` / `:LiveRunToggleNumbers` | Toggles line numbers on or off in the live output split window. |

### Keymap Example

Add keybindings to your Neovim configuration (`init.lua`):

```lua
-- Toggle LiveRunner with <leader>lr and stop with <leader>lq
vim.keymap.set("n", "<leader>lr", "<cmd>LiveRun<cr>", { desc = "Start Live Runner" })
vim.keymap.set("n", "<leader>lq", "<cmd>LiveRun stop<cr>", { desc = "Stop Live Runner" })
vim.keymap.set("n", "<leader>ln", "<cmd>LiveRunToggleNumbers<cr>", { desc = "Toggle Output Line Numbers" })
```

---

## ⚙️ Configuration

Pass a configuration table to `setup()` to override default settings:

```lua
require("live-runner").setup({
    port = 65432,             -- TCP port for the server to listen on
    bin_path = nil,           -- Custom path to the server binary (defaults to plugin src/server)
    show_line_numbers = false, -- Display line numbers in the output window (default: false)
})
```

### Configuration Options

| Option | Type | Default | Description |
|---|---|---|---|
| `port` | `number` | `65432` | TCP port used for communication between Neovim and the Go backend. |
| `bin_path` | `string|nil` | `nil` | Absolute path to custom `server` executable. If `nil`, auto-resolves to `src/server`. |
| `show_line_numbers` | `boolean` | `false` | Controls whether line numbers are displayed in the split output window. Default is `false` (off). |

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `NLR_TIMEOUT_MS` | `2000` | Code execution timeout in milliseconds before terminating long-running processes. |

Example:
```bash
export NLR_TIMEOUT_MS=5000  # Sets timeout to 5 seconds
```

---

## 🐛 Troubleshooting & FAQ

#### Q: Error `LiveRunner: Server binary not found at ...`
> **Solution**: The backend Go binary hasn't been compiled yet. Run `cd src && go build -o server` inside the plugin installation directory.

#### Q: Code output isn't updating as I type
> **Solution**: Ensure your file has a supported extension (`.py`, `.go`, `.lua`, or `.js`) and that the corresponding language runtime (`python3`, `go`, `lua`, `node`) is executable in your terminal `$PATH`.

#### Q: Port `65432` is already in use
> **Solution**: Update the port number in your setup configuration:
> ```lua
> require("live-runner").setup({ port = 54321 })
> ```

---

## 🛣️ Roadmap

- [ ] Support for temporary file execution across all interpreters.
- [ ] Configurable output split position (bottom, right, or floating window).
- [ ] Add support for Rust (`rustc`), C/C++ (`gcc`/`clang`), and TypeScript (`tsx`).
- [ ] Statusline component integration (`lualine.nvim`).

---

## ❤️ Contributing

Contributions, issues, and feature requests are welcome! Feel free to check the [issues page](https://github.com/shadowmkj/nvim-live-runner/issues).

---

## 📄 License

Distributed under the MIT License. See `LICENSE` for details.

Made with ❤️ by [shadowmkj](https://github.com/shadowmkj)
