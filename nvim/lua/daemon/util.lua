local M = {}

function M.split_text(text)
  return vim.split(text, "\n", { plain = true })
end

function M.advance_pos(row, col, text)
  local lines = M.split_text(text)
  if #lines == 1 then
    return row, col + #lines[1]
  end
  return row + #lines - 1, #lines[#lines]
end

function M.chunk_text(text, chars)
  chars = tonumber(chars) or 0
  if chars <= 0 or #text <= chars then
    return { text }
  end

  local chunks = {}
  local i = 1
  while i <= #text do
    local j = math.min(i + chars - 1, #text)
    local newline = text:find("\n", i, true)
    if newline and newline <= j then
      j = newline
    end
    table.insert(chunks, text:sub(i, j))
    i = j + 1
  end
  return chunks
end

function M.offset_to_pos(text, offset)
  local prefix = text:sub(1, offset)
  local _, row = prefix:gsub("\n", "")
  local last_newline = prefix:match(".*()\n")
  local col
  if last_newline then
    col = #prefix - last_newline
  else
    col = #prefix
  end
  return row, col
end

function M.minimal_replacement(old, new)
  local prefix = 0
  local max_prefix = math.min(#old, #new)
  while prefix < max_prefix and old:byte(prefix + 1) == new:byte(prefix + 1) do
    prefix = prefix + 1
  end

  local suffix = 0
  local max_suffix = math.min(#old - prefix, #new - prefix)
  while suffix < max_suffix and old:byte(#old - suffix) == new:byte(#new - suffix) do
    suffix = suffix + 1
  end

  local old_mid = old:sub(prefix + 1, #old - suffix)
  local new_mid = new:sub(prefix + 1, #new - suffix)
  return prefix, suffix, old_mid, new_mid
end

return M
