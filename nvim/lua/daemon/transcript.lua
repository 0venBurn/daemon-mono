local core = require("daemon.state")

local state = core.state
local notify = core.notify

local M = {}

local function ensure_workspace_session()
  if not state.workspace_session then
    state.workspace_session = {
      id = nil,
      title = "New session",
      transcript = {},
      context = {},
      active_operation = nil,
    }
  end
  state.workspace_session.title = state.workspace_session.title or "New session"
  state.workspace_session.transcript = state.workspace_session.transcript or {}
  state.workspace_session.context = state.workspace_session.context or {}
  return state.workspace_session
end

local function close_window()
  local win = state.transcript_win
  state.transcript_win = nil
  if win and vim.api.nvim_win_is_valid(win) then
    pcall(vim.api.nvim_win_close, win, true)
  end
end

local function ask_followup()
  local input_ui = require("daemon.input")
  input_ui.open({ prompt = "Daemon reply  (<C-s> submit, Enter submit in normal mode)" }, function(input)
    if not input or vim.trim(input) == "" then
      return
    end
    if state.actions and state.actions.submit_input then
      state.actions.submit_input("ask", input)
      return
    end
    notify("session input not available", vim.log.levels.WARN)
  end)
end

local function cancel_active()
  if state.actions and state.actions.cancel then
    state.actions.cancel("transcript")
  end
end

local function map_keys(buf)
  local opts = { buffer = buf, nowait = true, silent = true }
  vim.keymap.set("n", "q", close_window, vim.tbl_extend("force", opts, { desc = "Close daemon transcript" }))
  vim.keymap.set("n", "<Esc>", close_window, vim.tbl_extend("force", opts, { desc = "Close daemon transcript" }))
  vim.keymap.set("n", "<C-j>", "<C-d>", vim.tbl_extend("force", opts, { desc = "Scroll transcript down" }))
  vim.keymap.set("n", "<C-k>", "<C-u>", vim.tbl_extend("force", opts, { desc = "Scroll transcript up" }))
  vim.keymap.set("n", "J", "<C-d>", vim.tbl_extend("force", opts, { desc = "Scroll transcript down" }))
  vim.keymap.set("n", "K", "<C-u>", vim.tbl_extend("force", opts, { desc = "Scroll transcript up" }))
  vim.keymap.set("n", "a", ask_followup, vim.tbl_extend("force", opts, { desc = "Ask daemon follow-up" }))
  vim.keymap.set("n", "?", ask_followup, vim.tbl_extend("force", opts, { desc = "Ask daemon follow-up" }))
  vim.keymap.set("n", "c", cancel_active, vim.tbl_extend("force", opts, { desc = "Cancel daemon operation" }))
end

function M.ensure_workspace_session()
  return ensure_workspace_session()
end

local function ensure_buf()
  ensure_workspace_session()
  if not state.transcript_buf or not vim.api.nvim_buf_is_valid(state.transcript_buf) then
    state.transcript_buf = vim.api.nvim_create_buf(false, true)
    vim.bo[state.transcript_buf].buftype = "nofile"
    vim.bo[state.transcript_buf].bufhidden = "hide"
    vim.bo[state.transcript_buf].swapfile = false
    -- Keep this readable across themes. Markdown inline-code highlighting can
    -- create harsh dark boxes in floating windows, so use plain text for now.
    vim.bo[state.transcript_buf].filetype = "text"
    pcall(vim.api.nvim_buf_set_name, state.transcript_buf, "daemon://transcript")
    map_keys(state.transcript_buf)
  end
  return state.transcript_buf
end

local function display_title()
  local ws = ensure_workspace_session()
  return ws.title or "New session"
end

function M.open()
  local buf = ensure_buf()

  if not state.transcript_win or not vim.api.nvim_win_is_valid(state.transcript_win) then
    local width = math.max(36, math.min(78, vim.o.columns - 4))
    local height = math.max(8, math.min(18, vim.o.lines - 6))
    state.transcript_win = vim.api.nvim_open_win(buf, true, {
      relative = "editor",
      row = math.max(0, math.floor((vim.o.lines - height) / 2) - 1),
      col = math.max(0, math.floor((vim.o.columns - width) / 2)),
      width = width,
      height = height,
      style = "minimal",
      border = "rounded",
      title = " daemon: " .. display_title() .. " ",
      title_pos = "left",
    })
    vim.wo[state.transcript_win].wrap = true
    vim.wo[state.transcript_win].linebreak = true
    vim.wo[state.transcript_win].cursorline = true
    vim.wo[state.transcript_win].winhl = "NormalFloat:Normal,FloatBorder:FloatBorder"
  else
    vim.api.nvim_set_current_win(state.transcript_win)
    pcall(vim.api.nvim_win_set_config, state.transcript_win, { title = " daemon: " .. display_title() .. " " })
  end

  return buf
end

function M.close()
  close_window()
end

local function normalize_lines(lines)
  local normalized = {}
  for _, item in ipairs(lines or {}) do
    local text = tostring(item or "")
    local parts = vim.split(text, "\n", { plain = true })
    for _, part in ipairs(parts) do
      table.insert(normalized, part)
    end
  end
  return normalized
end

local function append_lines(lines)
  lines = normalize_lines(lines)
  local buf = ensure_buf()
  vim.bo[buf].modifiable = true
  local count = vim.api.nvim_buf_line_count(buf)
  local existing = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
  if count == 1 and existing[1] == "" then
    vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  else
    vim.api.nvim_buf_set_lines(buf, count, count, false, lines)
  end
  vim.bo[buf].modifiable = false
  if state.transcript_win and vim.api.nvim_win_is_valid(state.transcript_win) then
    pcall(vim.api.nvim_win_set_cursor, state.transcript_win, { vim.api.nvim_buf_line_count(buf), 0 })
  end
end

local function short_title(text)
  text = tostring(text or ""):gsub("[\r\n]+", " "):gsub("%s+", " ")
  text = vim.trim(text)
  if text == "" then
    return "New session"
  end
  if #text > 48 then
    return text:sub(1, 47) .. "…"
  end
  return text
end

function M.append_user(text)
  local ws = ensure_workspace_session()
  if not ws.title or ws.title == "New session" then
    ws.title = short_title(text)
    if state.transcript_win and vim.api.nvim_win_is_valid(state.transcript_win) then
      pcall(vim.api.nvim_win_set_config, state.transcript_win, { title = " daemon: " .. display_title() .. " " })
    end
  end
  table.insert(ws.transcript, { role = "user", text = text })
  append_lines({ "", "You", "", text })
end

function M.start_assistant()
  local ws = ensure_workspace_session()
  table.insert(ws.transcript, { role = "assistant", text = "" })
  append_lines({ "", "Daemon", "" })
end

function M.append_assistant_delta(text)
  if not text or text == "" then
    return
  end
  local ws = ensure_workspace_session()
  local last = ws.transcript[#ws.transcript]
  if not last or last.role ~= "assistant" then
    M.start_assistant()
    last = ws.transcript[#ws.transcript]
  end
  last.text = (last.text or "") .. text

  local buf = ensure_buf()
  local parts = vim.split(text, "\n", { plain = true })
  if #parts == 0 then
    return
  end
  vim.bo[buf].modifiable = true
  local line_count = vim.api.nvim_buf_line_count(buf)
  local line = vim.api.nvim_buf_get_lines(buf, line_count - 1, line_count, false)[1] or ""
  vim.api.nvim_buf_set_lines(buf, line_count - 1, line_count, false, { line .. parts[1] })
  if #parts > 1 then
    local rest = {}
    for i = 2, #parts do
      table.insert(rest, parts[i])
    end
    vim.api.nvim_buf_set_lines(buf, line_count, line_count, false, rest)
  end
  vim.bo[buf].modifiable = false
  if state.transcript_win and vim.api.nvim_win_is_valid(state.transcript_win) then
    pcall(vim.api.nvim_win_set_cursor, state.transcript_win, { vim.api.nvim_buf_line_count(buf), 0 })
  end
end

function M.append_status(_text)
  -- Transcript is conversation-only. Activity/status stays in tracker/notifications.
end

function M.append_edit_event(_text)
  -- Transcript is conversation-only. Raw patch/debug/edit details are not shown here.
end

function M.append_edit_block(_file, _row, _col, _text)
  -- Transcript is conversation-only. Applied edit text is intentionally omitted.
end

function M.reset()
  state.workspace_session = {
    id = nil,
    title = "New session",
    transcript = {},
    context = {},
    active_operation = nil,
  }
  if state.transcript_buf and vim.api.nvim_buf_is_valid(state.transcript_buf) then
    vim.bo[state.transcript_buf].modifiable = true
    vim.api.nvim_buf_set_lines(state.transcript_buf, 0, -1, false, {})
    vim.bo[state.transcript_buf].modifiable = false
  end
  if state.transcript_win and vim.api.nvim_win_is_valid(state.transcript_win) then
    pcall(vim.api.nvim_win_set_config, state.transcript_win, { title = " daemon: New session " })
  end
end

return M
