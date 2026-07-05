const rawAssetBase = normalizeAssetBase(
  (import.meta.env && import.meta.env.VITE_ASSET_BASE_URL) || '',
)

export function normalizeAssetBase(value = '') {
  const base = String(value).trim().replace(/\/+$/, '')
  if (!base) return ''
  if (base.startsWith('//')) return ''
  if (base.startsWith('/')) return base

  try {
    const url = new URL(base)
    return url.protocol === 'https:' ? url.toString().replace(/\/+$/, '') : ''
  } catch {
    return ''
  }
}

export function assetUrl(path) {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`
  if (!rawAssetBase) return normalizedPath
  return `${rawAssetBase}${normalizedPath}`
}
