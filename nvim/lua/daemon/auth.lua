local core = require("daemon.state")
local notify = core.notify

local M = {}

-- Provider definitions matching Go's llm.ProviderDetails
local providers = {
  {
    name = "anthropic",
    label = "󱚣 Anthropic Claude (direct)",
    env_key = "ANTHROPIC_API_KEY",
    default_model = "claude-sonnet-4-5",
    prompt = "Enter Anthropic API key (sk-ant-...)",
  },
  {
    name = "openai",
    label = "󰚩 OpenAI (direct)",
    env_key = "OPENAI_API_KEY",
    default_model = "gpt-4o",
    prompt = "Enter OpenAI API key (sk-...)",
  },
  {
    name = "google",
    label = "󰚩 Google Gemini (direct)",
    env_key = "GOOGLE_API_KEY",
    default_model = "gemini-2.0-flash",
    prompt = "Enter Google API key",
  },
  {
    name = "opencode",
    label = "󰚩 OpenCode Zen (proxy: Claude, DeepSeek, Qwen...)",
    env_key = "OPENCODE_API_KEY",
    default_model = "claude-sonnet-4-5",
    prompt = "Enter OpenCode API key",
  },
  {
    name = "opencode-go",
    label = "󰚩 OpenCode Go (proxy: DeepSeek, Kimi, MiMo...)",
    env_key = "OPENCODE_API_KEY",
    default_model = "deepseek-v4-flash",
    prompt = "Enter OpenCode API key",
  },
}

-- Read ~/.config/daemon/auth.json
local function read_auth_file()
  local config_dir = vim.fn.stdpath("config")
  -- Use XDG_CONFIG_HOME/daemon if available, otherwise ~/.config/daemon
  -- vim.fn.stdpath("config") is ~/.config/nvim, we want ~/.config/daemon
  local auth_dir = vim.fn.expand("$XDG_CONFIG_HOME")
  if auth_dir == "" or auth_dir == "$XDG_CONFIG_HOME" then
    auth_dir = vim.fn.expand("~/.config")
  end
  local auth_path = auth_dir .. "/daemon/auth.json"

  local f = io.open(auth_path, "r")
  if not f then
    return {}, auth_path
  end
  local content = f:read("*a")
  f:close()

  local ok, data = pcall(vim.json.decode, content)
  if not ok or type(data) ~= "table" then
    return {}, auth_path
  end
  return data, auth_path
end

-- Write ~/.config/daemon/auth.json
local function write_auth_file(data, auth_path)
  -- Ensure directory exists
  local dir = auth_path:match("^(.*)/")
  vim.fn.mkdir(dir, "p")

  -- Write with restricted permissions
  local f = io.open(auth_path, "w")
  if not f then
    notify("Failed to write auth file: " .. auth_path, vim.log.levels.ERROR)
    return false
  end
  f:write(vim.json.encode(data))
  f:close()

  -- Try to set permissions to 0600
  pcall(os.execute, "chmod 600 " .. vim.fn.shellescape(auth_path))
  return true
end

-- Set active provider in auth.json
local function set_active_provider(auth_path, provider_name)
  local data, path = read_auth_file()
  if path ~= auth_path then
    -- Use the discovered path
    auth_path = path
  end
  data["_active_provider"] = provider_name
  data["_active"] = nil
  return write_auth_file(data, auth_path)
end

-- Get current active provider name from auth.json
local function get_active_provider()
  local data, _ = read_auth_file()
  if data and data["_active_provider"] and data["_active_provider"] ~= "" then
    return data["_active_provider"]
  end
  if data and data["_active"] and data["_active"].api_key ~= "" then
    return data["_active"].api_key
  end
  -- Fall back to DAEMON_PROVIDER env var
  return os.getenv("DAEMON_PROVIDER") or "anthropic"
end

-- Get current auth status for display
local function get_auth_status()
  local data, auth_path = read_auth_file()
  local active = get_active_provider()
  local lines = {}
  local has_any_key = false

  for _, p in ipairs(providers) do
    local is_active = (p.name == active)
    local marker = is_active and " * " or "   "
    local has_key = data[p.name] and data[p.name].api_key and data[p.name].api_key ~= ""
    local model = data[p.name] and data[p.name].model or p.default_model
    local key_display = has_key and "✓ configured" or "✗ no key"
    has_any_key = has_any_key or has_key

    table.insert(lines, marker .. p.label)
    table.insert(lines, "     " .. key_display .. "  model: " .. model)
  end

  return lines, auth_path, data
end

-- Ask user for model name (optional override)
local function ask_model(callback, default_model)
  vim.ui.input({
    prompt = "Model (press Enter for default: " .. default_model .. "): ",
    default = "",
    completion = "",
  }, function(input)
    if input == nil then
      callback(nil) -- cancelled
    elseif vim.trim(input) == "" then
      callback(nil) -- use default
    else
      callback(vim.trim(input))
    end
  end)
end

-- Ask user for API key
local function ask_api_key(provider, callback)
  vim.ui.input({
    prompt = provider.prompt .. ": ",
    default = "",
    -- Don't show the key in command line history
  }, function(input)
    if input == nil or vim.trim(input) == "" then
      notify("Cancelled " .. provider.name .. " auth", vim.log.levels.WARN)
      callback(nil)
      return
    end
    callback(vim.trim(input))
  end)
end

-- Authenticate a provider: ask for key, save to auth.json, set active
local function auth_provider(provider)
  ask_api_key(provider, function(api_key)
    if not api_key then
      return
    end

    local data, auth_path = read_auth_file()

    -- Save provider entry
    data[provider.name] = {
      api_key = api_key,
      model = provider.default_model,
    }

    -- Set as active provider
    data["_active_provider"] = provider.name
    data["_active"] = nil

    if write_auth_file(data, auth_path) then
      -- Ask for model override
      ask_model(function(model_override)
        if model_override then
          data[provider.name].model = model_override
          write_auth_file(data, auth_path)
        end

        local restart_msg = "\nProvider set to: " .. provider.name
          .. "\nModel: " .. (model_override or provider.default_model)

        -- Set env var for current session so daemon restart picks it up
        vim.env[provider.env_key] = api_key
        vim.env.DAEMON_PROVIDER = provider.name
        vim.env.DAEMON_MODEL = model_override or provider.default_model

        notify("✓ Authenticated " .. provider.name .. restart_msg)

        -- Restart daemon with new credentials
        local rpc = require("daemon.rpc")
        local state = core.state
        if state.job and state.job > 0 then
          vim.fn.jobstop(state.job)
          state.job = nil
        end
        -- Next session start will use new env vars
        notify("Daemon will use new provider on next session start. Use :DaemonCancel to stop current session.", vim.log.levels.INFO)
      end, provider.default_model)
    end
  end)
end

-- Show provider list and let user pick one to authenticate
function M.auth()
  local lines, auth_path, data = get_auth_status()

  -- Build items for vim.ui.select
  local items = {}
  for i, p in ipairs(providers) do
    local has_key = data[p.name] and data[p.name].api_key and data[p.name].api_key ~= ""
    local active = (p.name == get_active_provider())
    local status = has_key and "✓" or "✗"
    local marker = active and "►" or " "
    table.insert(items, {
      provider = p,
      display = string.format("%s %s %-14s  %s  %s",
        marker, status, p.name, p.default_model, p.prompt:gsub("Enter ", "")),
    })
  end

  vim.ui.select(items, {
    prompt = "Select provider to authenticate:",
    format_item = function(item)
      return item.display
    end,
  }, function(choice)
    if not choice then
      return
    end
    auth_provider(choice.provider)
  end)
end

-- List current auth status
function M.status()
  local lines, auth_path, data = get_auth_status()

  local msg_lines = {
    "Daemon Auth Status",
    "Config: " .. auth_path,
    "",
  }
  for _, line in ipairs(lines) do
    table.insert(msg_lines, line)
  end

  local active = get_active_provider()
  table.insert(msg_lines, "")
  table.insert(msg_lines, "Active provider: " .. active)

  -- Check for env var overrides
  local env_provider = os.getenv("DAEMON_PROVIDER")
  local env_model = os.getenv("DAEMON_MODEL")
  if env_provider then
    table.insert(msg_lines, "DAEMON_PROVIDER=" .. env_provider .. " (env override)")
  end
  if env_model then
    table.insert(msg_lines, "DAEMON_MODEL=" .. env_model .. " (env override)")
  end

  -- Check for common API key env vars
  local env_keys = {
    "ANTHROPIC_API_KEY",
    "OPENAI_API_KEY",
    "GOOGLE_API_KEY",
    "OPENCODE_API_KEY",
  }
  for _, key in ipairs(env_keys) do
    local val = os.getenv(key)
    if val then
      table.insert(msg_lines, key .. "=" .. string.rep("*", #val) .. " (env)")
    end
  end

  notify(table.concat(msg_lines, "\n"))
end

-- Switch active provider (must already be authenticated)
function M.switch_provider()
  local data, auth_path = read_auth_file()
  local configured = {}

  for _, p in ipairs(providers) do
    if data[p.name] and data[p.name].api_key and data[p.name].api_key ~= "" then
      table.insert(configured, {
        provider = p,
        display = p.name .. " (" .. (data[p.name].model or p.default_model) .. ")",
      })
    end
  end

  if #configured == 0 then
    notify("No providers configured yet. Use :DaemonAuth to add one.", vim.log.levels.WARN)
    return
  end

  vim.ui.select(configured, {
    prompt = "Switch active provider:",
    format_item = function(item)
      return item.display
    end,
  }, function(choice)
    if not choice then
      return
    end
    data["_active_provider"] = choice.provider.name
    data["_active"] = nil
    if write_auth_file(data, auth_path) then
      vim.env.DAEMON_PROVIDER = choice.provider.name
      vim.env[choice.provider.env_key] = data[choice.provider.name].api_key
      notify("✓ Switched to " .. choice.provider.name)
    end
  end)
end

-- Logout: remove a provider's credentials
function M.logout()
  local data, auth_path = read_auth_file()
  local configured = {}

  for _, p in ipairs(providers) do
    if data[p.name] and data[p.name].api_key and data[p.name].api_key ~= "" then
      table.insert(configured, {
        provider = p,
        name = p.name,
      })
    end
  end

  if #configured == 0 then
    notify("No providers configured.", vim.log.levels.WARN)
    return
  end

  vim.ui.select(configured, {
    prompt = "Remove credentials for:",
    format_item = function(item)
      return item.name
    end,
  }, function(choice)
    if not choice then
      return
    end
    data[choice.name] = nil
    -- If removing active provider, clear active provider metadata.
    local active = get_active_provider()
    if active == choice.name then
      data["_active_provider"] = nil
      data["_active"] = nil
    end
    if write_auth_file(data, auth_path) then
      notify("✓ Removed " .. choice.name .. " credentials")
    end
  end)
end

-- Info: ask the daemon for its resolved provider configuration.
-- Falls back to a local approximation only when the daemon cannot start.
function M.info()
  local rpc = require("daemon.rpc")
  if rpc.ensure_daemon() and rpc.send("daemon/info", {}) then
    return
  end

  -- Fallback only if the daemon cannot start.
  local provider_name = vim.env.DAEMON_PROVIDER or "anthropic"
  local auth_data, auth_path = read_auth_file()

  -- Check auth.json active provider hint
  if not vim.env.DAEMON_PROVIDER or vim.env.DAEMON_PROVIDER == "" then
    if auth_data and auth_data["_active_provider"] and auth_data["_active_provider"] ~= "" then
      provider_name = auth_data["_active_provider"]
    elseif auth_data and auth_data["_active"] and auth_data["_active"].api_key ~= "" then
      provider_name = auth_data["_active"].api_key
    end
  end

  -- Resolve API key and base URL (same logic as Go LoadConfig)
  local api_key = ""
  local base_url = ""
  local model = vim.env.DAEMON_MODEL or ""
  local key_source = ""

  local function find_provider(name)
    for _, p in ipairs(providers) do
      if p.name == name then return p end
    end
    return nil
  end

  local p = find_provider(provider_name)

  -- Check env var first
  if p then
    local env_val = vim.env[p.env_key] or ""
    if env_val ~= "" then
      api_key = env_val
      key_source = "env:" .. p.env_key
    end
  end

  -- Check auth.json
  if api_key == "" and auth_data and auth_data[provider_name] then
    api_key = auth_data[provider_name].api_key or ""
    if api_key ~= "" then
      key_source = "auth.json"
    end
  end

  -- Check auth.json fallback keys
  if api_key == "" and provider_name == "opencode" then
    local fallback = vim.env.ANTHROPIC_API_KEY or ""
    if fallback ~= "" then
      api_key = fallback
      key_source = "env:ANTHROPIC_API_KEY (fallback)"
    end
  elseif api_key == "" and provider_name == "opencode-go" then
    local fallback = vim.env.OPENAI_API_KEY or ""
    if fallback ~= "" then
      api_key = fallback
      key_source = "env:OPENAI_API_KEY (fallback)"
    end
  end

  -- Resolve model
  if model == "" and auth_data and auth_data[provider_name] then
    model = auth_data[provider_name].model or ""
  end
  if model == "" and p then
    model = p.default_model
  end

  -- Resolve base URL
  if provider_name == "opencode" then
    base_url = "https://opencode.ai/zen"
    if vim.env.OPENCODE_BASE_URL and vim.env.OPENCODE_BASE_URL ~= "" then
      base_url = vim.env.OPENCODE_BASE_URL
    end
  elseif provider_name == "opencode-go" then
    base_url = "https://opencode.ai/zen/go/v1"
    if vim.env.OPENCODE_GO_BASE_URL and vim.env.OPENCODE_GO_BASE_URL ~= "" then
      base_url = vim.env.OPENCODE_GO_BASE_URL
    end
  elseif p then
    -- Direct providers use SDK defaults
    base_url = "(SDK default)"
  end

  -- Determine API type for opencode models
  local api_type = ""
  if provider_name == "anthropic" then
    api_type = "anthropic-messages"
  elseif provider_name == "openai" then
    api_type = "openai-completions"
  elseif provider_name == "google" then
    api_type = "google-generative-ai"
  elseif provider_name == "opencode" or provider_name == "opencode-go" then
    -- Determine from model
    if string.match(model, "^claude-") or model == "minimax-m3"
        or model == "qwen3.7-max" or model == "qwen3.7-plus"
        or model == "qwen3.6-plus" or model == "qwen3.5-plus" then
      api_type = "anthropic-messages"
      if provider_name == "opencode" then
        base_url = "https://opencode.ai/zen"
      else
        base_url = "https://opencode.ai/zen/go"
      end
    elseif string.match(model, "^gemini-") then
      api_type = "google-generative-ai"
    else
      api_type = "openai-completions"
    end
  end

  local key_display = ""
  if api_key ~= "" then
    if #api_key <= 8 then
      key_display = api_key
    else
      key_display = string.sub(api_key, 1, 4) .. "..." .. string.sub(api_key, -4)
    end
  else
    key_display = "(not set)"
  end

  local lines = {
    "╔══════════════════════════════════════╗",
    "║     Daemon Provider Configuration     ║",
    "╚══════════════════════════════════════╝",
    "",
    "  Provider:   " .. provider_name,
    "  Model:      " .. (model ~= "" and model or "(default)"),
    "  API type:   " .. (api_type ~= "" and api_type or "unknown"),
    "  Base URL:   " .. base_url,
    "  API key:    " .. key_display .. (key_source ~= "" and "  (" .. key_source .. ")" or ""),
    "",
    "  Auth file:  " .. auth_path,
    "  Env:        DAEMON_PROVIDER=" .. (vim.env.DAEMON_PROVIDER or "(unset)"),
  }

  notify(table.concat(lines, "\n"), vim.log.levels.INFO)
end

return M