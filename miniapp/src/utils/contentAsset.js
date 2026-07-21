import { API_BASE } from '../config'

function isSafeLocalPath(value, prefix) {
  if (!value.startsWith(prefix) || value.includes('\\')) return false
  try {
    return !decodeURIComponent(value).split(/[/?#]/).includes('..')
  } catch {
    return false
  }
}

function httpsApiOrigin() {
  try {
    const apiUrl = new URL(API_BASE)
    return apiUrl.protocol === 'https:' ? apiUrl.origin : ''
  } catch {
    return ''
  }
}

export function resolveContentAsset(value, fallback = '') {
  const safeFallback = typeof fallback === 'string' ? fallback : ''
  if (typeof value !== 'string') return safeFallback
  const asset = value.trim()
  if (!asset || asset.startsWith('//')) return safeFallback

  if (/^https:\/\//i.test(asset)) {
    try {
      const url = new URL(asset)
      return url.protocol === 'https:' ? asset : safeFallback
    } catch {
      return safeFallback
    }
  }

  if (isSafeLocalPath(asset, '/static/')) return asset
  if (!isSafeLocalPath(asset, '/assets/') && !isSafeLocalPath(asset, '/api/public/site-uploads/')) return safeFallback

  const origin = httpsApiOrigin()
  return origin ? new URL(asset, origin).href : safeFallback
}
