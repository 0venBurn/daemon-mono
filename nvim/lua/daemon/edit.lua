local core = require("daemon.state")
local buffer = require("daemon.buffer")
local ui = require("daemon.ui")
local util = require("daemon.util")
local transcript = require("daemon.transcript")
local tracker = require("daemon.tracker")

local state = core.state
local notify = core.notify

local M = {}

local process_patch_queue

local function call_action(name, ...)
  local fn = state.actions[name]
  if fn then
    return fn(...)
  end
end

local function animate_delete(buf, row, col, text, done)
  local s = state.session
  if not s then
    done(false)
    return
  end
  if text == "" then
    done(true)
    return
  end

  local chars = tonumber(state.config.stream_chars) or 0
  if chars <= 0 then
    chars = #text
  end
  local delay = tonumber(state.config.stream_delay_ms) or 0
  local remaining = text

  local function step()
    s = state.session
    if not s then
      done(false)
      return
    end
    if remaining == "" then
      s.row, s.col = row, col
      ui.update_ghost()
      done(true)
      return
    end

    local remove = math.min(chars, #remaining)
    local next_remaining = remaining:sub(1, #remaining - remove)
    local er, ec = util.advance_pos(row, col, remaining)

    state.applying = true
    pcall(vim.cmd, "silent! undojoin")
    local ok, err = pcall(vim.api.nvim_buf_set_text, buf, row, col, er, ec, util.split_text(next_remaining))
    state.applying = false
    if not ok then
      notify("patch delete animation failed: " .. tostring(err), vim.log.levels.ERROR)
      call_action("cancel", "patch delete animation failed")
      done(false)
      return
    end

    remaining = next_remaining
    s.row, s.col = util.advance_pos(row, col, remaining)
    ui.update_ghost()

    if delay > 0 then
      vim.defer_fn(step, delay)
    else
      step()
    end
  end

  step()
end

local function animate_insert(buf, row, col, text, done)
  local s = state.session
  if not s then
    done(false)
    return
  end
  if text == "" then
    done(true)
    return
  end

  local chunks = util.chunk_text(text, state.config.stream_chars)
  local i = 1
  local delay = tonumber(state.config.stream_delay_ms) or 0

  local function step()
    s = state.session
    if not s then
      done(false)
      return
    end
    if i > #chunks then
      done(true)
      return
    end

    local part = chunks[i]
    i = i + 1
    row, col = buffer.clamp_pos(buf, row, col)

    state.applying = true
    pcall(vim.cmd, "silent! undojoin")
    local ok, err = pcall(vim.api.nvim_buf_set_text, buf, row, col, row, col, util.split_text(part))
    state.applying = false
    if not ok then
      notify("patch animation failed: " .. tostring(err), vim.log.levels.ERROR)
      call_action("cancel", "patch animation failed")
      done(false)
      return
    end

    row, col = util.advance_pos(row, col, part)
    s.row, s.col = row, col
    ui.update_ghost()

    if delay > 0 then
      vim.defer_fn(step, delay)
    else
      step()
    end
  end

  step()
end

local function lines_lcs_hunks(old, new)
  local old_lines = util.split_text(old)
  local new_lines = util.split_text(new)
  local n, m = #old_lines, #new_lines
  local dp = {}
  for i = 0, n do
    dp[i] = {}
    for j = 0, m do
      dp[i][j] = 0
    end
  end
  for i = n - 1, 0, -1 do
    for j = m - 1, 0, -1 do
      if old_lines[i + 1] == new_lines[j + 1] then
        dp[i][j] = 1 + dp[i + 1][j + 1]
      else
        dp[i][j] = math.max(dp[i + 1][j], dp[i][j + 1])
      end
    end
  end

  local hunks = {}
  local i, j = 1, 1
  while i <= n or j <= m do
    if i <= n and j <= m and old_lines[i] == new_lines[j] then
      i = i + 1
      j = j + 1
    else
      local old_start, new_start = i, j
      while i <= n or j <= m do
        if i <= n and j <= m and old_lines[i] == new_lines[j] then
          break
        end
        if j > m or (i <= n and dp[i][j - 1] >= dp[i - 1][j]) then
          i = i + 1
        else
          j = j + 1
        end
      end
      table.insert(hunks, {
        old_start = old_start,
        old_end = i - 1,
        new_start = new_start,
        new_end = j - 1,
      })
    end
  end
  return old_lines, new_lines, hunks
end

local function line_offset(lines, first, last)
  local offset = 0
  for i = 1, first - 1 do
    offset = offset + #lines[i] + 1
  end
  local text = ""
  if first <= last then
    text = table.concat(vim.list_slice(lines, first, last), "\n")
  end
  return offset, text
end

local function hunk_jump_pos(row, col, old, new)
  row = row or 0
  col = col or 0
  if old ~= "" or new == "" then
    return row, col
  end
  local leading = tostring(new or ""):match("^%s*") or ""
  return util.advance_pos(row, col, leading)
end

local function hunk_label(file, row, col, old, new)
  local name = vim.fn.fnamemodify(file or "?", ":t")
  return "edit " .. name
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

local function tracker_mark(buf, row, col)
  if not buf or not vim.api.nvim_buf_is_valid(buf) then
    debug_log("tracker/extmark_failed", { reason = "invalid_buf", buf = buf, row = row, col = col })
    return nil
  end
  row, col = buffer.clamp_pos(buf, row or 0, col or 0)
  local ok, mark_id = pcall(vim.api.nvim_buf_set_extmark, buf, core.tracker_ns, row, col, {
    right_gravity = true,
  })
  if ok then
    debug_log("tracker/extmark_created", { buf = buf, mark_id = mark_id, row = row, col = col, file = vim.api.nvim_buf_get_name(buf) })
    return mark_id
  end
  debug_log("tracker/extmark_failed", { reason = tostring(mark_id), buf = buf, row = row, col = col })
  return nil
end

local function set_edit_mark(buf, row, col)
  local s = state.session
  if not s then
    return nil
  end
  if s.edit_mark_id and s.edit_mark_buf and vim.api.nvim_buf_is_valid(s.edit_mark_buf) then
    pcall(vim.api.nvim_buf_del_extmark, s.edit_mark_buf, core.edit_ns, s.edit_mark_id)
  end
  s.edit_mark_id = nil
  s.edit_mark_buf = nil
  if not buf or not vim.api.nvim_buf_is_valid(buf) then
    return nil
  end
  row, col = buffer.clamp_pos(buf, row or 0, col or 0)
  local ok, mark_id = pcall(vim.api.nvim_buf_set_extmark, buf, core.edit_ns, row, col, {
    right_gravity = true,
  })
  if ok then
    s.edit_mark_id = mark_id
    s.edit_mark_buf = buf
    return mark_id
  end
  return nil
end

local function get_edit_mark()
  local s = state.session
  if not s or not s.edit_mark_id or not s.edit_mark_buf then
    return nil, nil, nil
  end
  if not vim.api.nvim_buf_is_valid(s.edit_mark_buf) then
    return nil, nil, nil
  end
  local ok, pos = pcall(vim.api.nvim_buf_get_extmark_by_id, s.edit_mark_buf, core.edit_ns, s.edit_mark_id, {})
  if ok and pos and pos[1] then
    return s.edit_mark_buf, pos[1], pos[2]
  end
  return nil, nil, nil
end

local function clear_edit_mark()
  local s = state.session
  if not s then
    return
  end
  if s.edit_mark_id and s.edit_mark_buf and vim.api.nvim_buf_is_valid(s.edit_mark_buf) then
    pcall(vim.api.nvim_buf_del_extmark, s.edit_mark_buf, core.edit_ns, s.edit_mark_id)
  end
  s.edit_mark_id = nil
  s.edit_mark_buf = nil
end

local function add_edit_tracker(buf, file, row, col, old, new, show)
  local jump_row, jump_col = hunk_jump_pos(row, col, old, new)
  local mark_id = tracker_mark(buf, jump_row, jump_col)
  tracker.add("edit", hunk_label(file, row, col, old, new), {
    status = "✓",
    file = file,
    row = jump_row,
    col = jump_col,
    buf = buf,
    mark_id = mark_id,
    show = show,
  })
end

local function animate_hunks(buf, hunks, file, show_tracker, done)
  local i = 1
  local function record_and_continue(h)
    add_edit_tracker(buf, file, h.row, h.col, h.old, h.new, show_tracker)
    next_hunk()
  end
  function next_hunk()
    local h = hunks[i]
    i = i + 1
    if not h then
      done(true)
      return
    end
    local s = state.session
    if not s then
      done(false)
      return
    end
    s.row, s.col = h.row, h.col
    ui.update_ghost()
    if h.old == "" then
      animate_insert(buf, h.row, h.col, h.new, function(ok)
        if ok then
          record_and_continue(h)
        else
          done(false)
        end
      end)
      return
    end
    animate_delete(buf, h.row, h.col, h.old, function(deleted)
      if not deleted then
        done(false)
        return
      end
      animate_insert(buf, h.row, h.col, h.new, function(ok)
        if ok then
          record_and_continue(h)
        else
          done(false)
        end
      end)
    end)
  end
  next_hunk()
end

process_patch_queue = function()
  if state.patch_animating or not state.session then
    return
  end
  local params = table.remove(state.patch_queue, 1)
  if not params then
    M.maybe_finish_pending()
    return
  end

  local s = state.session
  if params.session_id and s.id and params.session_id ~= s.id then
    process_patch_queue()
    return
  end

  local target_file = params.file or s.file
  local buf = buffer.find_buf(target_file)
  if not buf and target_file then
    buf = vim.fn.bufadd(target_file)
    pcall(vim.fn.bufload, buf)
    -- Keep cross-file buffers alive for extmark tracking and jump-to
    if vim.api.nvim_buf_is_loaded(buf) then
      vim.bo[buf].buflisted = false
      vim.bo[buf].bufhidden = "hide"
    end
  end
  buf = buf or s.buf
  if not buf or not vim.api.nvim_buf_is_loaded(buf) then
    notify("target buffer not loaded: " .. (target_file or "?"), vim.log.levels.WARN)
    process_patch_queue()
    return
  end

  if params.op == "insert" then
    local row, col
    -- Use edit mark for position drift if available and on same buffer
    local mark_buf, mark_row, mark_col = get_edit_mark()
    if mark_buf and mark_buf == buf then
      row, col = mark_row, mark_col
    else
      row = tonumber(params.row) or s.row
      col = tonumber(params.col) or s.col
    end
    row, col = buffer.clamp_pos(buf, row, col)
    state.patch_animating = true
    ui.clear_marks()
    s.buf = buf
    s.file = params.file or s.file
    s.label = params.description or "insert"
    s.row, s.col = row, col
    transcript.append_edit_event(string.format("stream insert %s:%d:%d %s", s.file or "?", row + 1, col, s.label or ""))
    ui.update_ghost()
    animate_insert(buf, row, col, params.text or "", function(completed)
      state.patch_animating = false
      if completed then
        add_edit_tracker(buf, s.file, row, col, "", params.text or "", state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win))
        set_edit_mark(buf, s.row, s.col)
        process_patch_queue()
      end
    end)
    return
  end

  local old = params.old or ""
  local new = params.new or ""
  if old == "" or old == new then
    process_patch_queue()
    return
  end

  local text = buffer.buffer_text(buf)
  local start_idx, end_idx = text:find(old, 1, true)
  if not start_idx then
    -- If we have an edit mark on this buffer, the buffer may have shifted.
    -- Try again after reading fresh buffer text.
    text = buffer.buffer_text(buf)
    start_idx, end_idx = text:find(old, 1, true)
  end
  if not start_idx then
    local preview = old:sub(1, 80):gsub("\n", "\\n")
    local buf_name = vim.api.nvim_buf_get_name(buf)
    notify(string.format("patch skipped: old text not found in %s (%s...)", vim.fn.fnamemodify(buf_name, ":t"), preview), vim.log.levels.WARN)
    debug_log("patch/skip", { file = target_file, buf = buf, old_preview = preview, session_id = params.session_id })
    process_patch_queue()
    return
  end
  if text:find(old, end_idx + 1, true) then
    if state.config.allow_ambiguous_patches then
      notify("ambiguous patch: using first match", vim.log.levels.WARN)
    else
      notify(string.format("patch skipped: old text matched %d times in %s", 2, vim.fn.fnamemodify(vim.api.nvim_buf_get_name(buf), ":t")), vim.log.levels.WARN)
      debug_log("patch/ambiguous", { file = target_file, buf = buf, session_id = params.session_id })
      process_patch_queue()
      return
    end
  end

  state.patch_animating = true
  ui.clear_marks()
  s.buf = buf
  s.file = params.file or s.file
  s.label = params.description or "patch"

  if old:find("\n", 1, true) or new:find("\n", 1, true) then
    local old_lines, new_lines, line_hunks = lines_lcs_hunks(old, new)
    local visual_hunks = {}
    for h = #line_hunks, 1, -1 do
      local lh = line_hunks[h]
      local offset, old_hunk = line_offset(old_lines, lh.old_start, lh.old_end)
      local _, new_hunk = line_offset(new_lines, lh.new_start, lh.new_end)
      if lh.old_start > #old_lines and old_hunk == "" then
        offset = #old
        if new_hunk ~= "" then
          new_hunk = "\n" .. new_hunk
        end
      elseif old_hunk ~= "" and new_hunk ~= "" then
        local inner_prefix, _, old_mid, new_mid = util.minimal_replacement(old_hunk, new_hunk)
        offset = offset + inner_prefix
        old_hunk = old_mid
        new_hunk = new_mid
      end
      if old_hunk ~= "" or new_hunk ~= "" then
        local row, col = util.offset_to_pos(text, (start_idx - 1) + offset)
        row, col = buffer.clamp_pos(buf, row, col)
        table.insert(visual_hunks, { row = row, col = col, old = old_hunk, new = new_hunk })
      end
    end

    local first = visual_hunks[#visual_hunks] or visual_hunks[1]
    if first then
      s.row, s.col = first.row, first.col
    end
    transcript.append_edit_event(string.format("stream diff replace %s:%d:%d %s", s.file or "?", (s.row or 0) + 1, s.col or 0, s.label or ""))
    ui.update_ghost()

    animate_hunks(buf, visual_hunks, s.file, state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win), function(completed)
      state.patch_animating = false
      if completed then
        -- Place edit mark at end of last hunk so subsequent inserts drift correctly.
        local last = visual_hunks[#visual_hunks]
        if last then
          local final_row, final_col = util.advance_pos(last.row, last.col, last.new)
          final_row, final_col = buffer.clamp_pos(buf, final_row, final_col)
          set_edit_mark(buf, final_row, final_col)
        end
        process_patch_queue()
      end
    end)
    return
  end

  local prefix, suffix, old_mid, new_mid = util.minimal_replacement(old, new)
  local replace_start = (start_idx - 1) + prefix
  local replace_end = (start_idx - 1) + (#old - suffix)
  local sr, sc = util.offset_to_pos(text, replace_start)
  local er, ec = util.offset_to_pos(text, replace_end)
  sr, sc, er, ec = buffer.clamp_range(buf, sr, sc, er, ec)

  s.row, s.col = sr, sc
  transcript.append_edit_event(string.format("stream replace %s:%d:%d %s", s.file or "?", sr + 1, sc, s.label or ""))
  add_edit_tracker(buf, s.file, sr, sc, old_mid, new_mid, state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win))
  ui.update_ghost()

  animate_delete(buf, sr, sc, old_mid, function(deleted)
    if not deleted then
      state.patch_animating = false
      return
    end
    animate_insert(buf, sr, sc, new_mid, function(completed)
      state.patch_animating = false
      if completed then
        -- Place edit mark at the end of the inserted text so subsequent
        -- insert_at operations drift correctly with buffer changes.
        local final_row, final_col = util.advance_pos(sr, sc, new_mid)
        final_row, final_col = buffer.clamp_pos(buf, final_row, final_col)
        set_edit_mark(buf, final_row, final_col)
        process_patch_queue()
      end
    end)
  end)
end

function M.apply_replace(params)
  params.op = "replace"
  table.insert(state.patch_queue, params)
  process_patch_queue()
end

function M.apply_insert(params)
  params.op = "insert"
  table.insert(state.patch_queue, params)
  process_patch_queue()
end

function M.apply_create(params)
  local file = params.file
  if not file or file == "" then
    notify("create failed: missing file", vim.log.levels.ERROR)
    return
  end
  local buf = buffer.find_buf(file)
  if not buf then
    buf = vim.fn.bufadd(file)
  end
  pcall(vim.fn.bufload, buf)
  if not vim.api.nvim_buf_is_loaded(buf) then
    notify("create failed: could not load buffer " .. file, vim.log.levels.ERROR)
    return
  end
  vim.bo[buf].buftype = ""
  vim.bo[buf].bufhidden = "hide"
  vim.bo[buf].swapfile = true
  local lines = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
  if not (#lines == 1 and lines[1] == "") and #lines > 0 then
    notify("create skipped: buffer not empty " .. file, vim.log.levels.WARN)
    return
  end
  table.insert(state.patch_queue, {
    op = "insert",
    session_id = params.session_id,
    file = file,
    row = 0,
    col = 0,
    text = params.text or "",
    description = params.description or "create file",
  })
  process_patch_queue()
end

function M.apply_chunk(params)
  local s = state.session
  if not s then
    return
  end
  if params.session_id and s.id and params.session_id ~= s.id then
    return
  end

  local target_file = params.file or s.file
  local buf = buffer.find_buf(target_file)
  if not buf and target_file then
    buf = vim.fn.bufadd(target_file)
    pcall(vim.fn.bufload, buf)
    if vim.api.nvim_buf_is_loaded(buf) then
      vim.bo[buf].buflisted = false
      vim.bo[buf].bufhidden = "hide"
    end
  end
  buf = buf or s.buf
  if not buf or not vim.api.nvim_buf_is_loaded(buf) then
    notify("target buffer not loaded: " .. (target_file or "?"), vim.log.levels.WARN)
    return
  end

  ui.clear_marks()
  s.buf = buf
  s.file = params.file or s.file
  s.label = params.label or "writing"
  if not s.edit_start then
    s.edit_start = { row = s.row, col = s.col, file = s.file }
  end

  local text = params.text or ""
  if text == "" then
    return
  end

  local sr, sc, er, ec
  if s.pending_replace then
    sr, sc = s.pending_replace.start[1], s.pending_replace.start[2]
    er, ec = s.pending_replace["end"][1], s.pending_replace["end"][2]
    sr, sc, er, ec = buffer.clamp_range(buf, sr, sc, er, ec)
    s.pending_replace = nil
    set_edit_mark(buf, sr, sc)
  else
    local mark_buf
    mark_buf, sr, sc = get_edit_mark()
    -- Only use the mark if it's on the same buffer we're targeting.
    -- If the mark drifted to a different file (e.g., multi-file edit),
    -- fall back to session position.
    if mark_buf and mark_buf == buf then
      -- mark position is good, use it
    else
      set_edit_mark(buf, s.row, s.col)
      mark_buf, sr, sc = get_edit_mark()
      if not mark_buf then
        sr, sc = buffer.clamp_pos(buf, s.row, s.col)
      end
    end
    er, ec = sr, sc
  end

  local lines = util.split_text(text)
  state.applying = true
  pcall(vim.cmd, "silent! undojoin")
  local ok, err = pcall(vim.api.nvim_buf_set_text, buf, sr, sc, er, ec, lines)
  state.applying = false
  if not ok then
    notify("apply failed: " .. tostring(err), vim.log.levels.ERROR)
    call_action("cancel", "apply failed")
    return
  end

  s.row, s.col = util.advance_pos(sr, sc, text)
  s.edit_text = (s.edit_text or "") .. text
  set_edit_mark(buf, s.row, s.col)
  ui.update_ghost()
end

function M.maybe_finish_pending()
  if state.pending_done_buf and not state.patch_animating and #state.patch_queue == 0 then
    call_action("finish_session", state.pending_done_buf)
  end
end

function M.clear_edit_mark()
  clear_edit_mark()
end

return M
