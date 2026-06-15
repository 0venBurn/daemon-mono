local core = require("daemon.state")
local state = core.state
local notify = core.notify

local M = {}
local line_handler = nil

function M.set_line_handler(handler)
  line_handler = handler
end

function M.send(method, params)
  if not state.job or state.job <= 0 then
    return false
  end
  local msg = {
    id = state.next_id,
    method = method,
    params = params or {},
  }
  state.next_id = state.next_id + 1
  vim.fn.chansend(state.job, vim.json.encode(msg) .. "\n")
  return true
end

function M.ensure_daemon()
  if state.job and state.job > 0 then
    return true
  end

  if vim.fn.executable(state.config.cmd[1]) ~= 1 then
    notify("missing daemon: " .. state.config.cmd[1] .. " (run: go build -o ~/.local/bin/daemon ./cmd/daemon)", vim.log.levels.ERROR)
    return false
  end

  state.stdout = ""
  -- Build env: inherit parent env and override with daemon config + auth vars.
  -- jobstart's env field replaces the entire env when set, so we must
  -- include all relevant daemon/provider vars.
  local job_env = {
    DAEMON_STREAM_DELAY_MS = tostring(state.config.stream_delay_ms or 0),
    DAEMON_STREAM_CHARS = tostring(state.config.stream_chars or 0),
    DAEMON_PATCH_VALIDATION = tostring(state.config.patch_validation or "off"),
    DAEMON_ENABLE_FFF = state.config.enable_fff and "1" or (vim.env.DAEMON_ENABLE_FFF or "0"),
  }
  -- Forward provider env vars from Neovim's process env
  for _, key in ipairs({
    "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GOOGLE_API_KEY", "OPENCODE_API_KEY",
    "DAEMON_PROVIDER", "DAEMON_MODEL",
    "ANTHROPIC_BASE_URL", "OPENAI_BASE_URL", "GOOGLE_BASE_URL",
    "OPENCODE_BASE_URL", "OPENCODE_GO_BASE_URL", "DAEMON_DEBUG_FILE",
    "DAEMON_THINKING",
  }) do
    local val = vim.env[key]
    if val and val ~= "" then
      job_env[key] = val
    end
  end

  state.job = vim.fn.jobstart(state.config.cmd, {
    cwd = vim.loop.cwd(),
    env = job_env,
    stdout_buffered = false,
    stderr_buffered = false,
    on_stdout = function(_, data, _)
      if not data then
        return
      end
      local chunk = table.concat(data, "\n")
      if chunk == "" then
        return
      end
      state.stdout = state.stdout .. chunk
      while true do
        local idx = state.stdout:find("\n", 1, true)
        if not idx then
          break
        end
        local line = state.stdout:sub(1, idx - 1)
        state.stdout = state.stdout:sub(idx + 1)
        if line ~= "" and line_handler then
          vim.schedule(function()
            line_handler(line)
          end)
        end
      end
    end,
    on_stderr = function(_, data, _)
      local msg = table.concat(data or {}, "\n")
      if msg ~= "" then
        vim.schedule(function()
          notify(msg, vim.log.levels.WARN)
        end)
      end
    end,
    on_exit = function(_, code, _)
      vim.schedule(function()
        state.job = nil
        if state.session then
          local ui = require("daemon.ui")
          ui.clear_marks()
          state.session = nil
        end
        if code ~= 0 then
          notify("daemon exited: " .. code, vim.log.levels.WARN)
        end
      end)
    end,
  })

  if state.job <= 0 then
    notify("failed to start daemon", vim.log.levels.ERROR)
    state.job = nil
    return false
  end
  return true
end

return M
