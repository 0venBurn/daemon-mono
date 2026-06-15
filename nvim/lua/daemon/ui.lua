local core = require("daemon.state")
local buffer = require("daemon.buffer")

local state = core.state
local ns = core.ns
local notify = core.notify

local M = {}

function M.clear_marks()
  for _, buf in ipairs(vim.api.nvim_list_bufs()) do
    if vim.api.nvim_buf_is_valid(buf) and vim.api.nvim_buf_is_loaded(buf) then
      pcall(vim.api.nvim_buf_clear_namespace, buf, ns, 0, -1)
    end
  end
end

function M.update_ghost()
  local s = state.session
  if not s or not s.buf or not vim.api.nvim_buf_is_valid(s.buf) then
    return
  end
  local row, col = buffer.clamp_pos(s.buf, s.row, s.col)
  s.row, s.col = row, col
  vim.api.nvim_buf_clear_namespace(s.buf, ns, 0, -1)
  vim.api.nvim_buf_set_extmark(s.buf, ns, row, col, {
    id = 1,
    virt_text = { { "▌", "IncSearch" }, { " agent", "Comment" } },
    virt_text_pos = "inline",
    hl_mode = "combine",
  })
  vim.api.nvim_buf_set_extmark(s.buf, ns, row, 0, {
    id = 2,
    virt_lines = { { { "[writing] " .. vim.fn.fnamemodify(s.file, ":t") .. "  " .. (s.label or "streaming"), "Comment" } } },
    virt_lines_above = true,
  })
end

function M.render_plan(params)
  local lines = {}
  table.insert(lines, "Session: " .. (params.title or "agent edit"))
  table.insert(lines, "")
  for i, item in ipairs(params.items or {}) do
    table.insert(lines, string.format("%d. %s", i, item))
  end
  table.insert(lines, "")
  table.insert(lines, "<leader>aq / Esc: cancel")
  table.insert(lines, ":DaemonPrompt text: prompt/restart")

  if not state.plan_buf or not vim.api.nvim_buf_is_valid(state.plan_buf) then
    state.plan_buf = vim.api.nvim_create_buf(false, true)
    vim.bo[state.plan_buf].bufhidden = "wipe"
  end
  vim.api.nvim_buf_set_lines(state.plan_buf, 0, -1, false, lines)

  if not state.plan_win or not vim.api.nvim_win_is_valid(state.plan_win) then
    local width = 54
    local height = math.min(#lines + 2, 12)
    state.plan_win = vim.api.nvim_open_win(state.plan_buf, false, {
      relative = "editor",
      row = 1,
      col = vim.o.columns - width - 2,
      width = width,
      height = height,
      style = "minimal",
      border = "rounded",
      title = " daemon ",
      title_pos = "left",
    })
    vim.wo[state.plan_win].wrap = false
  end
end

function M.close_plan()
  local win = state.plan_win
  state.plan_win = nil
  if win and vim.api.nvim_win_is_valid(win) then
    vim.schedule(function()
      if vim.api.nvim_win_is_valid(win) then
        pcall(vim.api.nvim_win_close, win, true)
      end
    end)
  end
end

function M.close_explain_popup()
  local win = state.explain_win
  state.explain_win = nil
  if win and vim.api.nvim_win_is_valid(win) then
    pcall(vim.api.nvim_win_close, win, true)
  end
end

local function map_explain_keys(buf)
  local opts = { buffer = buf, nowait = true, silent = true }
  vim.keymap.set("n", "q", M.close_explain_popup, vim.tbl_extend("force", opts, { desc = "Close daemon explanation" }))
  vim.keymap.set("n", "<Esc>", M.close_explain_popup, vim.tbl_extend("force", opts, { desc = "Close daemon explanation" }))
  vim.keymap.set("n", "<C-j>", "<C-d>", vim.tbl_extend("force", opts, { desc = "Scroll explanation down" }))
  vim.keymap.set("n", "<C-k>", "<C-u>", vim.tbl_extend("force", opts, { desc = "Scroll explanation up" }))
  vim.keymap.set("n", "J", "<C-d>", vim.tbl_extend("force", opts, { desc = "Scroll explanation down" }))
  vim.keymap.set("n", "K", "<C-u>", vim.tbl_extend("force", opts, { desc = "Scroll explanation up" }))
end

function M.open_explain_popup()
  if not state.explain_buf or not vim.api.nvim_buf_is_valid(state.explain_buf) then
    state.explain_buf = vim.api.nvim_create_buf(false, true)
    vim.bo[state.explain_buf].buftype = "nofile"
    vim.bo[state.explain_buf].bufhidden = "hide"
    vim.bo[state.explain_buf].swapfile = false
    vim.bo[state.explain_buf].filetype = "markdown"
  end

  vim.bo[state.explain_buf].modifiable = true
  vim.api.nvim_buf_set_lines(state.explain_buf, 0, -1, false, { "" })
  vim.bo[state.explain_buf].modifiable = false

  if not state.explain_win or not vim.api.nvim_win_is_valid(state.explain_win) then
    local width = math.max(36, math.min(72, vim.o.columns - 4))
    local height = math.max(8, math.min(16, vim.o.lines - 6))
    state.explain_win = vim.api.nvim_open_win(state.explain_buf, true, {
      relative = "editor",
      row = math.max(0, math.floor((vim.o.lines - height) / 2) - 1),
      col = math.max(0, math.floor((vim.o.columns - width) / 2)),
      width = width,
      height = height,
      style = "minimal",
      border = "rounded",
      title = " daemon explain ",
      title_pos = "left",
    })
    vim.wo[state.explain_win].wrap = true
    vim.wo[state.explain_win].linebreak = true
    vim.wo[state.explain_win].cursorline = true
    map_explain_keys(state.explain_buf)
  else
    vim.api.nvim_set_current_win(state.explain_win)
  end

  return state.explain_buf
end

function M.append_explain_text(text)
  if not text or text == "" then
    return
  end
  local buf = state.explain_buf
  if not buf or not vim.api.nvim_buf_is_valid(buf) then
    buf = M.open_explain_popup()
  end

  local parts = vim.split(text, "\n", { plain = true })
  if #parts == 0 then
    return
  end

  vim.bo[buf].modifiable = true
  local line_count = vim.api.nvim_buf_line_count(buf)
  local last = vim.api.nvim_buf_get_lines(buf, line_count - 1, line_count, false)[1] or ""
  vim.api.nvim_buf_set_lines(buf, line_count - 1, line_count, false, { last .. parts[1] })
  if #parts > 1 then
    local rest = {}
    for i = 2, #parts do
      table.insert(rest, parts[i])
    end
    vim.api.nvim_buf_set_lines(buf, line_count, line_count, false, rest)
  end
  vim.bo[buf].modifiable = false

  if state.explain_win and vim.api.nvim_win_is_valid(state.explain_win) then
    local bottom = vim.api.nvim_buf_line_count(buf)
    pcall(vim.api.nvim_win_set_cursor, state.explain_win, { bottom, 0 })
  end
end

function M.diagnostics_status(buf)
  local diags = vim.diagnostic.get(buf)
  if #diags == 0 then
    notify("✓ diagnostics clean")
  else
    notify("✗ diagnostics: " .. #diags, vim.log.levels.WARN)
  end
end

return M
