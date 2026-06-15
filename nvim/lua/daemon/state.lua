local M = {}

M.ns = vim.api.nvim_create_namespace("daemon_agent")
M.tracker_ns = vim.api.nvim_create_namespace("daemon_tracker")
M.edit_ns = vim.api.nvim_create_namespace("daemon_edit_anchor")

M.state = {
  job = nil,
  stdout = "",
  next_id = 1,
  session = nil, -- compatibility alias for active_operation during transition
  workspace_session = {
    id = nil,
    title = "New session",
    transcript = {},
    context = {},
    active_operation = nil,
  },
  active_operation = nil,
  plan_buf = nil,
  plan_win = nil,
  transcript_buf = nil,
  transcript_win = nil,
  tracker_buf = nil,
  tracker_win = nil,
  tracker_events = {},
  tracker_index = 1,
  explain_buf = nil,
  explain_win = nil,
  applying = false,
  patch_queue = {},
  patch_animating = false,
  pending_done_buf = nil,
  actions = {},
  config = {
    cmd = { vim.fn.expand("~/.local/bin/daemon") },
    stream_delay_ms = 35,
    stream_chars = 3,
    patch_validation = "off",
    allow_ambiguous_patches = true,
    enable_fff = false,
  },
}

function M.notify(msg, level)
  vim.notify(msg, level or vim.log.levels.INFO, { title = "daemon" })
end

return M
