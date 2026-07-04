const rawAssetBase = ((import.meta.env && import.meta.env.VITE_ASSET_BASE_URL) || '').trim()

export function assetUrl(path) {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`
  if (!rawAssetBase) return normalizedPath
  return `${rawAssetBase.replace(/\/+$/, '')}${normalizedPath}`
}
