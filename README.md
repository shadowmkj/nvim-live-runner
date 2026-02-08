## Requirements
1. Go must be installed

## Setup

```return {
    "shadowmkj/nvim-live-runner",
    build = "cd src && go build -o server", -- This compiles the binary on install
    config = function()
        require("live-runner").setup({})
    end,
}```
