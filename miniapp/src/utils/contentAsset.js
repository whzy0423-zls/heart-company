import { API_BASE } from '../config'

const MAX_DECODE_PASSES = 6

function hasUnsafePathContent(value) {
  return /[\u0000-\u001f\u007f-\u009f]/.test(value)
    || value.includes('\\')
    || value.split(/[/?#]/).some((segment) => segment === '..')
}

function isSafelyEncodedPath(value) {
  let decoded = value
  for (let pass = 0; pass < MAX_DECODE_PASSES; pass += 1) {
    if (hasUnsafePathContent(decoded)) return false
    let next
    try {
      next = decodeURIComponent(decoded)
    } catch {
      return false
    }
    if (next === decoded) return true
    decoded = next
  }
  return false
}

function isSafeLocalPath(value, prefix) {
  return value.startsWith(prefix) && isSafelyEncodedPath(value)
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
