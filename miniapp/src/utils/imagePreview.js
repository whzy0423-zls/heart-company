function cleanImageUrl(value) {
  return typeof value === 'string' ? value.trim() : ''
}

export function previewImage(current, options = {}) {
  const currentUrl = cleanImageUrl(current)
  if (!currentUrl) return false

  const configuredUrls = Array.isArray(options.urls) ? options.urls : [currentUrl]
  const urls = [...new Set(configuredUrls.map(cleanImageUrl).filter(Boolean))]
  if (!urls.length) return false

  const preview = options.preview || globalThis?.uni?.previewImage
  if (typeof preview !== 'function') return false

  preview({
    current: currentUrl,
    urls,
  })
  return true
}
