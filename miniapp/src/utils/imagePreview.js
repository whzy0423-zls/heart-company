function cleanImageUrl(value) {
  return typeof value === 'string' ? value.trim() : ''
}

export function isWechatDevtools() {
  try {
    // The native `wx.previewImage` bridge exists on real devices as well as
    // in developer tools, so it cannot identify the simulator by itself.
    // Prefer the explicit platform marker and only use the bridge as a
    // compatibility fallback when no platform information is available.
    const platform = globalThis?.uni?.getSystemInfoSync?.().platform
    if (platform) return platform === 'devtools'
    return typeof globalThis?.wx?.previewImage === 'function'
  } catch {
    return false
  }
}

export function previewImage(current, options = {}) {
  const currentUrl = cleanImageUrl(current)
  if (!currentUrl) return false

  const configuredUrls = Array.isArray(options.urls) ? options.urls : [currentUrl]
  const urls = [...new Set(configuredUrls.map(cleanImageUrl).filter(Boolean))]
  if (!urls.length) return false

  const preview = options.preview || globalThis?.uni?.previewImage
  if (typeof preview !== 'function') return false

  const previewOptions = {
    current: currentUrl,
    urls,
  }

  // WeChat's native preview API is stricter than the <image> component: a
  // bundled `/static/...` path may render inline but be rejected by
  // `previewImage`. Resolve that local asset to a temporary file first. Tests
  // can inject `preview`, so they continue to observe the synchronous call.
  const getImageInfo = globalThis?.uni?.getImageInfo
  const isBundledAsset = /^\/(?:static|assets)\//i.test(currentUrl)
  if (!options.preview && isBundledAsset && typeof getImageInfo === 'function') {
    try {
      getImageInfo({
        src: currentUrl,
        success(info = {}) {
          const resolved = cleanImageUrl(info.path || info.tempFilePath)
          if (!resolved) {
            preview(previewOptions)
            return
          }
          preview({
            current: resolved,
            urls: urls.map((url) => (url === currentUrl ? resolved : url)),
          })
        },
        fail() {
          preview(previewOptions)
        },
      })
    } catch {
      preview(previewOptions)
    }
    return true
  }

  preview(previewOptions)
  return true
}
