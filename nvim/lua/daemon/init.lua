local core = require("daemon.state")
local rpc = require("daemon.rpc")
local protocol = require("daemon.protocol")
local session = require("daemon.session")
local ui = require("daemon.ui")
local transcript = require("daemon.transcript")
local tracker = require("daemon.tracker")
local auth = require("daemon.auth")

local state = core.state

local M = {}

M.edit = session.edit
M.explain = session.explain
M.prompt = session.prompt
M.ask = session.ask
M.cancel = session.cancel
M.new_session = session.new_session
M.status = session.status
M.set_throttle = session.set_throttle
M._handle_line = protocol.handle_line
M._clear_marks = ui.clear_marks
M.activity = tracker.open

local function create_commands()
  vim.api.nvim_create_user_command("DaemonEdit", function(opts)
    if opts.args and vim.trim(opts.args) ~= "" then
      session.prompt(opts.args)
    else
      session.edit(false)
    end
  end, { nargs = "*" })

  vim.api.nvim_create_user_command("DaemonPrompt", function(opts)
    session.prompt(opts.args)
  end, { nargs = "*" })

  vim.api.nvim_create_user_command("DaemonAsk", function(opts)
    session.ask(opts.args)
  end, { nargs = "*" })

  vim.api.nvim_create_user_command("DaemonNewSession", function()
    session.new_session()
  end, {})

  vim.api.nvim_create_user_command("DaemonCancel", function()
    session.cancel("command")
  end, {})

  vim.api.nvim_create_user_command("DaemonExplain", function(opts)
    session.explain(opts.range and opts.range > 0)
  end, { range = true })

  vim.api.nvim_create_user_command("DaemonExplainClose", function()
    transcript.close()
  end, {})

  vim.api.nvim_create_user_command("DaemonStatus", function()
    session.status()
  end, {})

  vim.api.nvim_create_user_command("DaemonThrottle", function(opts)
    session.set_throttle(opts.args)
  end, { nargs = "+" })

  vim.api.nvim_create_user_command("Daemon", function(opts)
    session.prompt(opts.args)
  end, { nargs = "*" })

  vim.api.nvim_create_user_command("DaemonActivity", function()
    tracker.open()
  end, {})

  vim.api.nvim_create_user_command("DaemonAuth", function()
    auth.auth()
  end, {})

  vim.api.nvim_create_user_command("DaemonAuthStatus", function()
    auth.status()
  end, {})

  vim.api.nvim_create_user_command("DaemonAuthSwitch", function()
    auth.switch_provider()
  end, {})

  vim.api.nvim_create_user_command("DaemonAuthLogout", function()
    auth.logout()
  end, {})

  vim.api.nvim_create_user_command("DaemonInfo", function()
    auth.info()
  end, {})

  vim.api.nvim_create_user_command("DaemonThinking", function(opts)
    local level = vim.trim(opts.args or "")
    if level == "" then
      -- No arg: show current level
      local rpc = require("daemon.rpc")
      if rpc.ensure_daemon() then
        rpc.send("daemon/info", {})
      end
      return
    end
    local valid = { off = true, low = true, medium = true, high = true, xhigh = true }
    if not valid[level] then
      local core = require("daemon.state")
      core.notify("Invalid thinking level: " .. level .. ". Use: off, low, medium, high, xhigh", vim.log.levels.ERROR)
      return
    end
    local rpc = require("daemon.rpc")
    if rpc.ensure_daemon() then
      rpc.send("daemon/set_thinking", { level = level })
    end
  end, { nargs = "?", complete = function() return { "off", "low", "medium", "high", "xhigh" } end })
end

local function create_keymaps()
  vim.keymap.set("n", "<leader>ai", function()
    session.edit(false)
  end, { desc = "Daemon agent edit" })

  vim.keymap.set("x", "<leader>ai", function()
    session.edit(true)
  end, { desc = "Daemon agent edit selection" })

  vim.keymap.set("x", "<leader>ax", function()
    session.explain(true)
  end, { desc = "Daemon explain selection" })

  vim.keymap.set("n", "<leader>ax", function()
    session.explain(false)
  end, { desc = "Daemon explain selection" })

  vim.keymap.set("n", "<leader>aq", function()
    session.cancel("key")
  end, { desc = "Cancel daemon agent" })

  vim.keymap.set("n", "<Esc>", function()
    if state.session then
      vim.schedule(function()
        session.cancel("Esc")
      end)
      return ""
    end
    if state.transcript_win and vim.api.nvim_win_is_valid(state.transcript_win) then
      vim.schedule(transcript.close)
      return ""
    end
    return "<Esc>"
  end, { expr = true, desc = "Cancel daemon agent, close explanation, or Esc" })
end

function M.setup(opts)
  if opts then
    state.config = vim.tbl_deep_extend("force", state.config, opts)
  end

  rpc.set_line_handler(protocol.handle_line)
  if state.did_setup then
    return
  end
  state.did_setup = true
  create_commands()
  create_keymaps()
end

return M
