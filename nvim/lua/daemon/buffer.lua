local core = require("daemon.state")
local notify = core.notify

local M = {}

function M.absname(buf)
  return vim.fn.fnamemodify(vim.api.nvim_buf_get_name(buf), ":p")
end

function M.find_buf(file)
  local abs = vim.fn.fnamemodify(file, ":p")
  for _, buf in ipairs(vim.api.nvim_list_bufs()) do
    if vim.api.nvim_buf_is_loaded(buf) and M.absname(buf) == abs then
      return buf
    end
  end
  return nil
end

function M.line_len(buf, row)
  local line = vim.api.nvim_buf_get_lines(buf, row, row + 1, false)[1] or ""
  return #line
end

function M.clamp_pos(buf, row, col)
  local line_count = vim.api.nvim_buf_line_count(buf)
  row = math.max(0, math.min(row, line_count - 1))
  local line = vim.api.nvim_buf_get_lines(buf, row, row + 1, false)[1] or ""
  col = math.max(0, math.min(col, #line))
  return row, col
end

function M.clamp_range(buf, sr, sc, er, ec)
  local line_count = vim.api.nvim_buf_line_count(buf)
  sr = math.max(0, math.min(sr, line_count - 1))
  er = math.max(0, math.min(er, line_count - 1))
  if er < sr then
    sr, er = er, sr
    sc, ec = ec, sc
  end
  sc = math.max(0, math.min(sc, M.line_len(buf, sr)))
  ec = math.max(0, math.min(ec, M.line_len(buf, er)))
  if er == sr and ec < sc then
    sc, ec = ec, sc
  end
  return sr, sc, er, ec
end

function M.buffer_text(buf)
  return table.concat(vim.api.nvim_buf_get_lines(buf, 0, -1, false), "\n")
end

function M.selection_from_visual(buf)
  local s = vim.fn.getpos("'<")
  local e = vim.fn.getpos("'>")
  local mode = vim.fn.visualmode()
  local sr, sc = s[2] - 1, s[3] - 1
  local er, ec = e[2] - 1, e[3]

  if er < sr or (er == sr and ec < sc) then
    sr, er = er, sr
    sc, ec = ec, sc
  end

  if mode == "V" then
    sc = 0
    ec = M.line_len(buf, er)
  end

  sr, sc, er, ec = M.clamp_range(buf, sr, sc, er, ec)
  local text = table.concat(vim.api.nvim_buf_get_text(buf, sr, sc, er, ec, {}), "\n")
  return {
    start = { sr, sc },
    ["end"] = { er, ec },
    text = text,
  }
end

function M.attach_interrupt(buf, cancel)
  vim.api.nvim_buf_attach(buf, false, {
    on_lines = function()
      -- allow concurrent editing; patches skip gracefully when stale
    end,
    on_detach = function()
      if core.state.session then
        cancel("buffer detached")
      end
    end,
  })
end

function M.require_named_current_buffer()
  local buf = vim.api.nvim_get_current_buf()
  local file = M.absname(buf)
  if file == "" then
    notify("buffer has no file name", vim.log.levels.ERROR)
    return nil, nil
  end
  return buf, file
end

return M
