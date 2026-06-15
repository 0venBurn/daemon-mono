local core = require("daemon.state")
local buffer = require("daemon.buffer")
local transcript = require("daemon.transcript")

local state = core.state

local M = {}

local spinner_frames = { "⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏" }

local function has_loading_events()
  for _, ev in ipairs(state.tracker_events or {}) do
    if ev.loading then
      return true
    end
  end
  return false
end

local function stop_spinner_if_idle()
  if has_loading_events() then
    return
  end
  if state.tracker_spinner_timer then
    state.tracker_spinner_timer:stop()
    state.tracker_spinner_timer:close()
    state.tracker_spinner_timer = nil
  end
end

local function start_spinner()
  if state.tracker_spinner_timer then
    return
  end
  state.tracker_spinner_index = state.tracker_spinner_index or 1
  state.tracker_spinner_timer = vim.loop.new_timer()
  state.tracker_spinner_timer:start(0, 120, vim.schedule_wrap(function()
    if not has_loading_events() then
      stop_spinner_if_idle()
      return
    end
    state.tracker_spinner_index = (state.tracker_spinner_index % #spinner_frames) + 1
    for _, ev in ipairs(state.tracker_events or {}) do
      if ev.loading then
        ev.status = spinner_frames[state.tracker_spinner_index]
      end
    end
    if state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) then
      M.render()
    end
  end))
end

local function ensure_state()
  state.tracker_events = state.tracker_events or {}
  state.tracker_index = state.tracker_index or 1
end

local function single_line(text)
  text = tostring(text or "")
  text = text:gsub("[\r\n]+", " / ")
  text = text:gsub("%s+", " ")
  return vim.trim(text)
end

local function ellipsize(text, max)
  text = single_line(text)
  max = max or 72
  if #text <= max then
    return text
  end
  return text:sub(1, max - 1) .. "…"
end

local function event_line(ev, i)
  local mark = i == state.tracker_index and "→" or " "
  local status = ev.status or "·"
  local width = 42
  if state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) then
    width = math.max(24, vim.api.nvim_win_get_width(state.tracker_win) - 6)
  end
  local text = ellipsize(ev.text or ev.kind or "event", width)
  return string.format("%s %s %s", mark, status, text)
end

local function visible_range()
  local count = #state.tracker_events
  if count == 0 then
    return 1, 0
  end
  local height = 9
  if state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) then
    -- Header is 2 lines, footer is 2 lines. Use the rest for events.
    height = math.max(1, vim.api.nvim_win_get_height(state.tracker_win) - 4)
  end
  local start = math.max(1, state.tracker_index - height + 1)
  start = math.min(start, math.max(1, count - height + 1))
  return start, math.min(count, start + height - 1)
end

function M.render()
  ensure_state()
  if not state.tracker_buf or not vim.api.nvim_buf_is_valid(state.tracker_buf) then
    state.tracker_buf = vim.api.nvim_create_buf(false, true)
    vim.bo[state.tracker_buf].buftype = "nofile"
    vim.bo[state.tracker_buf].bufhidden = "hide"
    vim.bo[state.tracker_buf].swapfile = false
    vim.bo[state.tracker_buf].filetype = "text"
    pcall(vim.api.nvim_buf_set_name, state.tracker_buf, "daemon://activity")
  end

  local lines = {}
  local ws = state.workspace_session or {}
  table.insert(lines, "󰚩 Daemon")
  table.insert(lines, "session " .. (ws.id or "local"))
  table.insert(lines, "")
  if #state.tracker_events == 0 then
    table.insert(lines, "idle")
  else
    local start, finish = visible_range()
    state.tracker_render_start = start
    state.tracker_render_finish = finish
    for i = start, finish do
      table.insert(lines, event_line(state.tracker_events[i], i))
    end
  end
  table.insert(lines, "")
  table.insert(lines, "↵ jump   i inspect   c cancel   q close")

  vim.bo[state.tracker_buf].modifiable = true
  vim.api.nvim_buf_set_lines(state.tracker_buf, 0, -1, false, lines)
  vim.api.nvim_buf_clear_namespace(state.tracker_buf, -1, 0, -1)
  vim.api.nvim_buf_add_highlight(state.tracker_buf, -1, "Title", 0, 0, -1)
  vim.api.nvim_buf_add_highlight(state.tracker_buf, -1, "Comment", 1, 0, -1)
  vim.api.nvim_buf_add_highlight(state.tracker_buf, -1, "Comment", #lines - 1, 0, -1)
  for lnum = 3, #lines - 2 do
    local line = lines[lnum + 1] or ""
    if line:find("✓", 1, true) then
      vim.api.nvim_buf_add_highlight(state.tracker_buf, -1, "DiagnosticOk", lnum, 2, 5)
    elseif line:find("→", 1, true) then
      vim.api.nvim_buf_add_highlight(state.tracker_buf, -1, "Identifier", lnum, 0, 1)
    end
  end
  vim.bo[state.tracker_buf].modifiable = false

  if not state.tracker_win or not vim.api.nvim_win_is_valid(state.tracker_win) then
    local current = vim.api.nvim_get_current_win()
    vim.cmd("botright 42vsplit")
    state.tracker_win = vim.api.nvim_get_current_win()
    vim.api.nvim_win_set_buf(state.tracker_win, state.tracker_buf)
    vim.wo[state.tracker_win].wrap = false
    vim.wo[state.tracker_win].linebreak = false
    vim.wo[state.tracker_win].breakindent = false
    vim.wo[state.tracker_win].showbreak = ""
    vim.wo[state.tracker_win].cursorline = true
    vim.wo[state.tracker_win].number = false
    vim.wo[state.tracker_win].relativenumber = false
    vim.wo[state.tracker_win].signcolumn = "no"
    vim.wo[state.tracker_win].winfixwidth = true
    if current and vim.api.nvim_win_is_valid(current) then
      vim.api.nvim_set_current_win(current)
    end

    local opts = { buffer = state.tracker_buf, nowait = true, silent = true }
    vim.keymap.set("n", "i", function() transcript.open() end, vim.tbl_extend("force", opts, { desc = "Open daemon transcript" }))
    vim.keymap.set("n", "c", function()
      if state.actions and state.actions.cancel then state.actions.cancel("tracker") end
    end, vim.tbl_extend("force", opts, { desc = "Cancel daemon operation" }))
    vim.keymap.set("n", "q", function() M.close() end, vim.tbl_extend("force", opts, { desc = "Close daemon activity" }))
    vim.keymap.set("n", "<CR>", function() M.jump() end, vim.tbl_extend("force", opts, { desc = "Jump to daemon event" }))
    vim.keymap.set("n", "j", function() M.move(1) end, vim.tbl_extend("force", opts, { desc = "Next daemon event" }))
    vim.keymap.set("n", "k", function() M.move(-1) end, vim.tbl_extend("force", opts, { desc = "Previous daemon event" }))
    vim.keymap.set("n", "<Down>", function() M.move(1) end, vim.tbl_extend("force", opts, { desc = "Next daemon event" }))
    vim.keymap.set("n", "<Up>", function() M.move(-1) end, vim.tbl_extend("force", opts, { desc = "Previous daemon event" }))
  end
end

function M.open()
  local current = vim.api.nvim_get_current_win()
  if not (state.tracker_win and current == state.tracker_win) then
    state.tracker_prev_win = current
  end
  M.render()
  if state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) then
    vim.api.nvim_set_current_win(state.tracker_win)
  end
end

function M.close()
  local win = state.tracker_win
  state.tracker_win = nil
  if win and vim.api.nvim_win_is_valid(win) then
    pcall(vim.api.nvim_win_close, win, true)
  end
end

function M.stop_loading(kind)
  ensure_state()
  for _, ev in ipairs(state.tracker_events) do
    if ev.loading and (not kind or ev.kind == kind) then
      ev.loading = false
      ev.status = "·"
    end
  end
  stop_spinner_if_idle()
  if state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) then
    M.render()
  end
end

function M.add(kind, text, opts)
  ensure_state()
  opts = opts or {}
  local loading = opts.loading or false
  if opts.complete_kind then
    for i = #state.tracker_events, 1, -1 do
      local ev = state.tracker_events[i]
      if ev.kind == opts.complete_kind and ev.loading then
        ev.loading = false
        ev.status = opts.status or "✓"
        ev.text = text or ev.text
        ev.file = opts.file or ev.file
        ev.row = opts.row or ev.row
        ev.col = opts.col or ev.col
        stop_spinner_if_idle()
        if opts.show ~= false then
          M.render()
        end
        return
      end
    end
  end
  local ev = {
    kind = kind,
    text = text,
    status = loading and spinner_frames[state.tracker_spinner_index or 1] or (opts.status or "·"),
    loading = loading,
    file = opts.file,
    row = opts.row,
    col = opts.col,
    buf = opts.buf,
    mark_id = opts.mark_id,
  }
  table.insert(state.tracker_events, ev)
  state.tracker_index = #state.tracker_events
  if loading then
    start_spinner()
  else
    stop_spinner_if_idle()
  end
  if opts.show ~= false then
    M.render()
  end
end

function M.move(delta)
  ensure_state()
  if #state.tracker_events == 0 then return end
  state.tracker_index = math.max(1, math.min(#state.tracker_events, state.tracker_index + delta))
  M.render()
  if state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) then
    local start = visible_range()
    local line = 4 + (state.tracker_index - start)
    pcall(vim.api.nvim_win_set_cursor, state.tracker_win, { line, 0 })
  end
end

local function target_editor_win()
  if state.tracker_prev_win and vim.api.nvim_win_is_valid(state.tracker_prev_win) then
    return state.tracker_prev_win
  end
  for _, win in ipairs(vim.api.nvim_list_wins()) do
    local buf = vim.api.nvim_win_get_buf(win)
    if buf ~= state.tracker_buf and buf ~= state.transcript_buf then
      return win
    end
  end
  return nil
end

local function debug_log(kind, payload)
  local debug_file = vim.env.DAEMON_DEBUG_FILE
  if not debug_file or debug_file == "" then
    debug_file = "/tmp/daemon-debug.jsonl"
  end
  payload = payload or {}
  payload.kind = kind
  payload.source = "nvim"
  payload.cwd = vim.loop.cwd()
  local ok, line = pcall(vim.json.encode, payload)
  if ok then
    pcall(vim.fn.writefile, { line }, debug_file, "a")
  end
end

function M.jump()
  ensure_state()
  if state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) then
    local cursor = vim.api.nvim_win_get_cursor(state.tracker_win)
    local start = state.tracker_render_start or 1
    local idx = start + cursor[1] - 4
    if state.tracker_events[idx] then
      state.tracker_index = idx
    end
  end
  local ev = state.tracker_events[state.tracker_index]
  if not ev or not ev.file then return end
  local win = target_editor_win()
  if win then
    vim.api.nvim_set_current_win(win)
  end
  local existing = buffer.find_buf(ev.file)
  if existing then
    vim.api.nvim_win_set_buf(0, existing)
  else
    vim.cmd.edit(vim.fn.fnameescape(ev.file))
  end
  -- Re-resolve buffer after opening — bufadd buffers may load here
  local buf = buffer.find_buf(ev.file)
  local row, col = ev.row, ev.col
  local mark_buf = ev.buf
  if not mark_buf or not vim.api.nvim_buf_is_valid(mark_buf) then
    -- Original buffer gone; try the newly-resolved one
    mark_buf = buf
  end
  if mark_buf and ev.mark_id then
    local pos = vim.api.nvim_buf_get_extmark_by_id(mark_buf, core.tracker_ns, ev.mark_id, {})
    debug_log("tracker/jump_extmark_lookup", { buf = mark_buf, mark_id = ev.mark_id, fallback_row = ev.row, fallback_col = ev.col, pos = pos, file = ev.file })
    if pos and #pos == 2 then
      row, col = pos[1], pos[2]
    end
  else
    debug_log("tracker/jump_no_extmark", { has_buf = ev.buf ~= nil, mark_id = ev.mark_id, row = ev.row, col = ev.col, file = ev.file })
  end
  if row then
    pcall(vim.api.nvim_win_set_cursor, 0, { (row or 0) + 1, col or 0 })
  end
end

return M
