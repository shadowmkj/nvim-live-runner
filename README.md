# nvim-live-runner 🚀

A simple, real-time code runner for Neovim that gives you instant feedback on your code. Write Python and see the results live, right in your editor!

<!-- ![demo](https://user-images.githubusercontent.com/8619876/184493322-132d74c3-b50a-4299-a417-03ea54832502.gif) -->
<!-- *(Demo GIF is a placeholder, but illustrates the concept)* -->

This plugin is in its early stages. Currently, it only supports **Python**, but the goal is to expand to more languages soon.

## ✨ Features

*   **Live Feedback:** Get instant results from your Python code as you type.
*   **Simple & Lightweight:** No complex setup. Just install and go.
*   **Go-powered Backend:** A fast and reliable backend server to execute your code.

## ✅ Requirements

*   [Go](https://go.dev/doc/install) (v1.18 or higher) must be installed on your system for the backend server.
*   Neovim >= 0.7

## 📦 Installation

Install with your favorite plugin manager.

### [lazy.nvim](https://github.com/folke/lazy.nvim)

```lua
return {
    "shadowmkj/nvim-live-runner",
    build = "cd src && go build -o server", -- This compiles the binary on install
    config = function()
        require("live-runner").setup({})
    end,
}
```

### [packer.nvim](https://github.com/wbthomason/packer.nvim)

```lua
use {
    "shadowmkj/nvim-live-runner",
    run = "cd src && go build -o server",
    config = function()
        require("live-runner").setup({})
    end,
}
```

## 🚀 Usage

1.  Open a Python file (`.py`).
2.  Run the command `:LiveRun`.
3.  This will open a new split window to show the output of your code.
4.  Start coding! Any changes you make will be executed automatically, and the output window will update in real-time.

## ⚙️ Configuration

You can pass a configuration table to the `setup()` function. Here are the default values:

```lua
require("live-runner").setup({
    port = 65432, -- The port for the server to listen on
    bin_path = nil, -- Path to the server binary. Defaults to the one built by the plugin.
})
```

## 🛣️ Roadmap

*   [ ] Support for more languages (JavaScript, Ruby, etc.)
*   [ ] More robust error handling.
*   [ ] Customizable output window layout.

## ❤️ Contributing

Contributions, issues, and feature requests are welcome!

---

Made with ❤️ by shadowmkj
