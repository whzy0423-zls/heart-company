const DEFAULT_STATUS_BAR_HEIGHT = 44
const DEFAULT_CAPSULE_INSET = 104
const CAPSULE_GAP = 4

function finitePositive(value) {
  const number = Number(value)
  return Number.isFinite(number) && number > 0 ? number : 0
}

function read(api, method) {
  try {
    const fn = api?.[method]
    return typeof fn === 'function' ? fn.call(api) || {} : {}
  } catch {
    return {}
  }
}

/**
 * Resolve the dimensions needed by a custom WeChat navigation bar.
 * The explicit `wechat` flag keeps H5 previews independent of device profile
 * strings (H5 on an iPhone still reports platform=ios in some runtimes).
 */
export function resolveHomeNavigation(api, { wechat = false } = {}) {
  if (!wechat) return { statusBarHeight: 0, capsuleInset: 0 }

  let modern = {}
  try {
    const value = api?.getWindowInfo
    if (typeof value === 'function') modern = value.call(api) || {}
  } catch {
    modern = {}
  }

  const modernStatus = finitePositive(modern.statusBarHeight)
  const modernSafe = finitePositive(modern.safeArea?.top) || finitePositive(modern.safeAreaInsets?.top)

  // Avoid the deprecated API when the modern API already supplied a status
  // bar value, but continue independently if the modern bridge is absent or
  // throws.
  let legacy = {}
  if (!modernStatus && !modernSafe) legacy = read(api, 'getSystemInfoSync')
  const legacyStatus = finitePositive(legacy.statusBarHeight)
  const legacySafe = finitePositive(legacy.safeArea?.top) || finitePositive(legacy.safeAreaInsets?.top)

  let capsule = {}
  try {
    const value = api?.getMenuButtonBoundingClientRect
    if (typeof value === 'function') capsule = value.call(api) || {}
  } catch {
    capsule = {}
  }
  const capsuleTop = finitePositive(capsule.top)
  const capsuleStatus = finitePositive(capsuleTop - CAPSULE_GAP)
  const statusBarHeight = Math.round(modernStatus || modernSafe || legacyStatus || legacySafe || capsuleStatus || DEFAULT_STATUS_BAR_HEIGHT)

  const width = finitePositive(modern.windowWidth) || finitePositive(legacy.windowWidth)
  const left = finitePositive(capsule.left)
  const inset = width > 0 && left > 0 && left < width ? width - left + 12 : 0
  return {
    statusBarHeight,
    capsuleInset: Math.round(inset > 0 ? inset : DEFAULT_CAPSULE_INSET),
  }
}
