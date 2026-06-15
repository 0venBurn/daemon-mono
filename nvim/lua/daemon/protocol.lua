local core = require("daemon.state")
local ui = require("daemon.ui")
local edit = require("daemon.edit")
local session = require("daemon.session")
local transcript = require("daemon.transcript")
local tracker = require("daemon.tracker")

local state = core.state
local notify = core.notify

local function display_path(path)
  if not path or path == "" then
    return "."
  end
  return vim.fn.fnamemodify(path, ":~:.")
end

local function short_result(text)
  text = tostring(text or "")
  if text == "" then
    return ""
  end
  local lines = {}
  for line in text:gmatch("[^\r\n]+") do
    table.insert(lines, line)
    if #lines >= 2 then
      break
    end
  end
  local suffix = ""
  local count = select(2, text:gsub("\n", "")) + 1
  if count > #lines then
    suffix = string.format(" (+%d lines)", count - #lines)
  end
  local summary = table.concat(lines, " / ") .. suffix
  summary = summary:gsub("%s+", " ")
  if #summary > 72 then
    summary = summary:sub(1, 71) .. "…"
  end
  return summary
end

local function tool_args(params)
  local args = params.arguments
  if type(args) == "string" then
    local ok, decoded = pcall(vim.json.decode, args)
    if ok then args = decoded end
  end
  return type(args) == "table" and args or {}
end

local function tool_label(name, params)
  params = params or {}
  local args = tool_args(params)
  local file = params.file or args.file or args.path or ""
  local path = display_path(file)
  local short = file ~= "" and vim.fn.fnamemodify(file, ":t") or ""
  if name == "grep" then
    local q = args.query or args.pattern or "?"
    local target = args.file or args.path or params.file or "."
    return string.format("grep /%s/ in %s", tostring(q), display_path(target))
  elseif name == "find_files" then
    return "find " .. tostring(args.query or args.pattern or "")
  elseif name == "read_file" then
    return "read " .. path
  elseif name == "list_directory" then
    return "ls " .. path
  elseif name == "replace_text" then
    return (params.description and params.description ~= "") and ("edit " .. short .. " — " .. params.description) or ("edit " .. path)
  elseif name == "insert_at" then
    local line = params.row and (":" .. tostring((params.row or 0) + 1)) or ""
    return (params.description and params.description ~= "") and ("insert " .. short .. line .. " — " .. params.description) or ("insert " .. path .. line)
  elseif name == "create_file" then
    return "create " .. path
  elseif name == "write_file" then
    return "write " .. path
  elseif name == "delete_file" then
    return "delete " .. path
  end
  return short ~= "" and (name .. " " .. path) or name
end

local M = {}

function M.handle_line(line)
  local ok, msg = pcall(vim.json.decode, line)
  if not ok then
    notify("bad daemon json: " .. line, vim.log.levels.ERROR)
    return
  end

  local method = msg.method
  local params = msg.params or {}

  if method == "daemon/info" then
    local key = params.has_key and "set" or "not set"
    local thinking = params.thinking and params.thinking ~= "" and (" thinking=" .. params.thinking) or ""
    notify(string.format("provider=%s model=%s api=%s base_url=%s key=%s%s", tostring(params.provider or "?"), tostring(params.model or "?"), tostring(params.api_type or "?"), tostring(params.base_url or "(default)"), key, thinking))
  elseif method == "session/start" then
    if state.session then
      state.session.id = params.session_id
    end
    if state.active_operation then
      state.active_operation.id = params.session_id
    end
    if state.workspace_session then
      state.workspace_session.id = params.session_id
    end
  elseif method == "plan/update" then
    tracker.add("plan", params.title or "plan update", { status = "·", show = state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) })
  elseif method == "session/status" then
    if state.session then
      state.session.label = params.message or params.state
      ui.update_ghost()
    end
    local message = params.message or params.state or "status"
    if message:find("^calling model") then
      tracker.add("model", "processing", { loading = true, show = state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) })
    elseif message:find("^model response received") then
      tracker.add("model", "processing", { status = "✓", complete_kind = "model", show = state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) })
    elseif params.state == "cancelled" or params.state == "done" or params.state == "prompted" then
      tracker.stop_loading()
    end
  elseif method == "edit/chunk" then
    edit.apply_chunk(params)
  elseif method == "edit/replace" then
    edit.apply_replace(params)
  elseif method == "edit/insert" then
    edit.apply_insert(params)
  elseif method == "edit/create" then
    edit.apply_create(params)
  elseif method == "explain/chunk" then
    transcript.append_assistant_delta(params.text or "")
  elseif method == "session/done" then
    tracker.stop_loading()
    if state.active_operation and (state.active_operation.kind == "explain" or state.active_operation.kind == "ask") then
      session.finish_explain_session()
      return
    end
    local buf = state.session and state.session.buf
    if state.patch_animating or #state.patch_queue > 0 then
      state.pending_done_buf = buf
    else
      session.finish_session(buf)
    end
  elseif method == "tool/start" then
    local name = params.name or params.tool or "tool"
    local text = tool_label(name, params)
    tracker.add("tool", text, { loading = true, file = params.file, row = params.row, col = params.col, show = state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) })
  elseif method == "tool/done" then
    local name = params.name or params.tool or "tool"
    local text = tool_label(name, params)
    local summary = short_result(params.result)
    if summary ~= "" and (name == "grep" or name == "find_files" or name == "read_file" or name == "list_directory") then
      text = text .. " → " .. summary
    end
    tracker.add("tool", text, { status = "✓", complete_kind = "tool", file = params.file, row = params.row, col = params.col, show = state.tracker_win and vim.api.nvim_win_is_valid(state.tracker_win) })
  elseif method == "debug/invalid_patch_json" then
    transcript.append_status("invalid patch JSON attempt " .. tostring(params.attempt or "?") .. ": " .. tostring(params.error or "") .. " debug_file=" .. tostring(params.debug_file or ""))
    if params.raw and params.raw ~= "" then
      transcript.append_edit_event("raw invalid JSON\n" .. params.raw)
    end
  elseif method == "debug/invalid_patch" then
    transcript.append_status("invalid patch attempt " .. tostring(params.attempt or "?") .. ": " .. tostring(params.error or "") .. " debug_file=" .. tostring(params.debug_file or ""))
    if params.raw and params.raw ~= "" then
      transcript.append_edit_event("raw rejected patch\n" .. params.raw)
    end
  elseif method == "debug/raw_patch" then
    transcript.append_edit_event("raw patch attempt " .. tostring(params.attempt or "?") .. "\n" .. tostring(params.text or ""))
  elseif method == "session/error" or method == "daemon/error" then
    tracker.stop_loading()
    local err = params.error or msg.error or "daemon error"
    if params.details and params.details ~= "" then
      err = err .. "\n" .. params.details
    end
    if params.raw and params.raw ~= "" then
      err = err .. "\nraw: " .. params.raw
    end
    notify(err, vim.log.levels.ERROR)
  end
end

return M
