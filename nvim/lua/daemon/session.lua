local core = require("daemon.state")
local rpc = require("daemon.rpc")
local buffer = require("daemon.buffer")
local ui = require("daemon.ui")
local transcript = require("daemon.transcript")
local tracker = require("daemon.tracker")
local input = require("daemon.input")
local edit = require("daemon.edit")

local state = core.state
local notify = core.notify

local M = {}

local function stop_fff_background_monitor()
  -- Do not `require` fff here. If it is loaded and watching, stop its native
  -- background watcher before daemon starts mutating buffers/files.
  local file_picker = package.loaded["fff.file_picker"]
  if file_picker and file_picker.stop_background_monitor then
    pcall(file_picker.stop_background_monitor)
  end

  local fuzzy = package.loaded["fff.fuzzy"]
  if fuzzy and fuzzy.stop_background_monitor then
    pcall(fuzzy.stop_background_monitor)
  end
end

local function current_context(visual)
  local buf, file = buffer.require_named_current_buffer()
  if not buf then
    return nil
  end
  local cursor = vim.api.nvim_win_get_cursor(0)
  local row, col = cursor[1] - 1, cursor[2]
  local selection = nil
  if visual then
    selection = buffer.selection_from_visual(buf)
    if selection then
      row, col = selection.start[1], selection.start[2]
      pcall(vim.api.nvim_win_set_cursor, 0, { row + 1, col })
    end
  end
  return {
    buf = buf,
    file = file,
    row = row,
    col = col,
    selection = selection,
    content = buffer.buffer_text(buf),
    filetype = vim.bo[buf].filetype,
    changedtick = vim.b[buf].changedtick,
  }
end

local function submit_input(intent, text, ctx)
  if not text or vim.trim(text) == "" then
    return
  end
  if not rpc.ensure_daemon() then
    return
  end
  stop_fff_background_monitor()
  ctx = ctx or current_context(false)
  if not ctx then
    return
  end

  local ws = transcript.ensure_workspace_session()
  ws.context = vim.tbl_deep_extend("force", ws.context or {}, ctx)
  transcript.append_user(text)
  if intent == "explain" or intent == "ask" then
    transcript.start_assistant()
    transcript.open()
  end
  tracker.add("operation", intent, { status = "→", file = ctx.file, row = ctx.row, col = ctx.col, show = state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) })

  state.active_operation = {
    id = ws.id,
    kind = intent,
    buf = ctx.buf,
    file = ctx.file,
    row = ctx.row,
    col = ctx.col,
    prompt = text,
    label = intent,
  }
  ws.active_operation = state.active_operation
  state.session = state.active_operation
  buffer.attach_interrupt(ctx.buf, M.cancel)
  ui.update_ghost()

  rpc.send("session/input", {
    session_id = ws.id,
    intent = intent,
    text = text,
    cwd = vim.loop.cwd(),
    context = {
      file = ctx.file,
      filetype = ctx.filetype,
      cursor = { ctx.row, ctx.col },
      selection = ctx.selection,
      content = ctx.content,
      changedtick = ctx.changedtick,
      transcript = ws.transcript,
    },
  })
end

local function start_edit(prompt, visual)
  if state.session then
    M.cancel("new operation")
  end
  local intent = "edit"
  local text = prompt or ""
  submit_input(intent, text, current_context(visual))
end

function M.edit(visual)
  input.open({ prompt = "Daemon edit  (<C-s> submit, Enter submit in normal mode)" }, function(text)
    start_edit(text, visual)
  end)
end

function M.explain(visual)
  if not visual then
    notify("select code first", vim.log.levels.WARN)
    return
  end
  if state.session then
    M.cancel("new operation")
  end
  local ctx = current_context(true)
  if not ctx or not ctx.selection or vim.trim(ctx.selection.text or "") == "" then
    notify("selection is empty", vim.log.levels.WARN)
    return
  end
  submit_input("explain", "Explain the selected code.", ctx)
end

function M.ask(args)
  local text = args or ""
  local function ask_context()
    if state.workspace_session and state.workspace_session.context and state.workspace_session.context.file then
      return state.workspace_session.context
    end
    return current_context(false)
  end
  if vim.trim(text) ~= "" then
    submit_input("ask", text, ask_context())
    return
  end
  input.open({ prompt = "Daemon reply  (<C-s> submit, Enter submit in normal mode)" }, function(reply)
    submit_input("ask", reply, ask_context())
  end)
end

function M.prompt(args)
  if args and vim.trim(args) ~= "" then
    if state.session then
      local old = vim.deepcopy(state.session)
      local prompt = old.prompt .. "\n\nPrompt update: " .. args
      M.cancel("prompt")
      vim.defer_fn(function()
        if old.buf and vim.api.nvim_buf_is_valid(old.buf) then
          vim.api.nvim_set_current_buf(old.buf)
          local row, col = buffer.clamp_pos(old.buf, old.row, old.col)
          pcall(vim.api.nvim_win_set_cursor, 0, { row + 1, col })
        end
        start_edit(prompt, false)
      end, 80)
    else
      start_edit(args, false)
    end
    return
  end
  M.edit(false)
end

function M.new_session()
  if state.session and state.session.id then
    rpc.send("session/cancel", { session_id = state.session.id })
  elseif state.job then
    rpc.send("session/cancel", { session_id = "" })
  end
  state.patch_queue = {}
  state.patch_animating = false
  state.pending_done_buf = nil
  state.active_operation = nil
  state.session = nil
  state.tracker_events = {}
  state.tracker_index = 1
  ui.clear_marks()
  edit.clear_edit_mark()
  transcript.reset()
  if state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) then
    tracker.render()
  end
end

function M.cancel(reason)
  if state.session and state.session.id then
    rpc.send("session/cancel", { session_id = state.session.id })
  elseif state.job then
    rpc.send("session/cancel", { session_id = "" })
  end
  state.patch_queue = {}
  state.patch_animating = false
  state.pending_done_buf = nil
  ui.clear_marks()
  edit.clear_edit_mark()
  if state.active_operation and state.active_operation.kind == "explain" then
    transcript.append_status("cancelled")
  end
  state.active_operation = nil
  if state.workspace_session then
    state.workspace_session.active_operation = nil
  end
  state.session = nil
  if reason then
    notify("cancelled: " .. reason)
  end
end

function M.finish_explain_session()
  ui.clear_marks()
  edit.clear_edit_mark()
  transcript.append_status("operation done")
  state.active_operation = nil
  if state.workspace_session then
    state.workspace_session.active_operation = nil
  end
  state.session = nil
  notify("explanation done")
end

local function notify_diagnostics_later(buf)
  if not buf or not vim.api.nvim_buf_is_valid(buf) then
    notify("done")
    return
  end
  local target_tick = vim.b[buf].changedtick
  vim.defer_fn(function()
    if not vim.api.nvim_buf_is_valid(buf) then
      return
    end
    vim.defer_fn(function()
      if not vim.api.nvim_buf_is_valid(buf) then
        return
      end
      if vim.b[buf].changedtick ~= target_tick then
        return
      end
      ui.diagnostics_status(buf)
    end, 250)
  end, 500)
end

function M.finish_session(buf)
  local s = state.session
  if s and s.edit_text and s.edit_text ~= "" then
    local start = s.edit_start or { row = s.row, col = s.col, file = s.file }
    transcript.append_edit_block(start.file or s.file, start.row or 0, start.col or 0, s.edit_text)
  end
  transcript.append_status("operation done")
  tracker.add("operation", "done", { status = "✓", file = s and s.file, row = s and s.row, col = s and s.col })
  ui.clear_marks()
  edit.clear_edit_mark()
  state.active_operation = nil
  if state.workspace_session then
    state.workspace_session.active_operation = nil
  end
  state.session = nil
  state.patch_queue = {}
  state.patch_animating = false
  state.pending_done_buf = nil
  notify_diagnostics_later(buf)
end

function M.status()
  if state.session then
    notify("active: " .. (state.session.id or "starting") .. " " .. state.session.file)
  else
    notify("idle")
  end
end

function M.set_throttle(args)
  local parts = vim.split(args or "", "%s+", { trimempty = true })
  local ms = tonumber(parts[1])
  local chars = tonumber(parts[2]) or state.config.stream_chars
  if not ms or ms < 0 or not chars or chars < 0 then
    notify("usage: :DaemonThrottle <ms> [chars_per_chunk]", vim.log.levels.ERROR)
    return
  end
  state.config.stream_delay_ms = ms
  state.config.stream_chars = chars
  if state.session then
    M.cancel("throttle changed")
  end
  if state.job and state.job > 0 then
    vim.fn.jobstop(state.job)
    state.job = nil
  end
  notify("stream speed: " .. ms .. "ms per " .. chars .. " chars")
end

function M.submit_input(intent, text)
  local ctx = nil
  if intent == "ask" and state.workspace_session and state.workspace_session.context and state.workspace_session.context.file then
    ctx = state.workspace_session.context
  else
    ctx = current_context(false) or (state.workspace_session and state.workspace_session.context)
  end
  submit_input(intent, text, ctx)
end

state.actions.cancel = M.cancel
state.actions.submit_input = M.submit_input
state.actions.finish_session = M.finish_session
state.actions.new_session = M.new_session

return M
