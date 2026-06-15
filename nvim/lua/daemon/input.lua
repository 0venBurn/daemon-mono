local M = {}

local function measure(buf)
  local lines = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
  local max_len = 0
  for _, line in ipairs(lines) do
    max_len = math.max(max_len, #line)
  end
  local width = math.max(60, math.min(vim.o.columns - 8, max_len + 6))
  local height = math.max(3, math.min(12, #lines + 2))
  return width, height
end

function M.open(opts, on_submit)
  opts = opts or {}
  local title = opts.prompt or "Daemon prompt"
  local buf = vim.api.nvim_create_buf(false, true)
  vim.bo[buf].buftype = "prompt"
  vim.bo[buf].bufhidden = "wipe"
  vim.bo[buf].swapfile = false
  vim.bo[buf].filetype = "text"
  vim.fn.prompt_setprompt(buf, "")
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, { "" })

  local width, height = measure(buf)
  local win = vim.api.nvim_open_win(buf, true, {
    relative = "editor",
    row = math.max(1, math.floor((vim.o.lines - height) / 3)),
    col = math.max(0, math.floor((vim.o.columns - width) / 2)),
    width = width,
    height = height,
    style = "minimal",
    border = "rounded",
    title = " " .. title .. " ",
    title_pos = "left",
  })
  vim.wo[win].wrap = true
  vim.wo[win].linebreak = true
  vim.wo[win].winhl = "NormalFloat:Normal,FloatBorder:FloatBorder"

  local function close()
    if vim.api.nvim_win_is_valid(win) then
      pcall(vim.api.nvim_win_close, win, true)
    end
  end

  local function submit()
    if not vim.api.nvim_buf_is_valid(buf) then
      return
    end
    local text = table.concat(vim.api.nvim_buf_get_lines(buf, 0, -1, false), "\n")
    close()
    if on_submit then
      on_submit(text)
    end
  end

  local function resize()
    if not vim.api.nvim_win_is_valid(win) or not vim.api.nvim_buf_is_valid(buf) then
      return
    end
    local w, h = measure(buf)
    vim.api.nvim_win_set_config(win, {
      relative = "editor",
      row = math.max(1, math.floor((vim.o.lines - h) / 3)),
      col = math.max(0, math.floor((vim.o.columns - w) / 2)),
      width = w,
      height = h,
    })
  end

  local map_opts = { buffer = buf, silent = true, nowait = true }
  vim.keymap.set({ "n", "i" }, "<C-s>", submit, vim.tbl_extend("force", map_opts, { desc = "Submit daemon prompt" }))
  vim.keymap.set("n", "<CR>", submit, vim.tbl_extend("force", map_opts, { desc = "Submit daemon prompt" }))
  vim.keymap.set({ "n", "i" }, "<Esc>", close, vim.tbl_extend("force", map_opts, { desc = "Cancel daemon prompt" }))

  vim.api.nvim_create_autocmd({ "TextChanged", "TextChangedI" }, {
    buffer = buf,
    callback = resize,
  })

  vim.cmd.startinsert()
end

return M
