const DEFAULT_PREVIEW_LIMIT = 3

export function previewItems(items, limit = DEFAULT_PREVIEW_LIMIT) {
  return Array.isArray(items) ? items.slice(0, limit) : []
}

export function hiddenCount(items, limit = DEFAULT_PREVIEW_LIMIT) {
  if (!Array.isArray(items) || items.length <= limit) return 0
  return items.length - limit
}
