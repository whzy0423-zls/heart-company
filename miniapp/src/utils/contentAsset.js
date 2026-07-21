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

function validPort(value) {
  if (!value) return true
  if (!/^[1-9]\d{0,4}$/.test(value)) return false
  return Number(value) <= 65535
}

function validHostname(value) {
  if (!value || value.length > 253 || value.startsWith('.') || value.endsWith('.')) return false
  const labels = value.split('.')
  if (labels.some((label) => !/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/i.test(label))) return false
  if (labels.length === 4 && labels.every((label) => /^\d+$/.test(label))) {
    return labels.every((label) => Number(label) <= 255)
  }
  return true
}

function httpsOrigin(value) {
  if (typeof value !== 'string' || /[\s\u0000-\u001f\u007f-\u009f\\]/.test(value)) return ''
  const match = /^https:\/\/([^/?#]+)(?=\/|\?|#|$)/i.exec(value)
  if (!match) return ''
  const authority = match[1]
  if (authority.includes('@')) return ''

  const authorityMatch = /^([^:]+)(?::([^:]+))?$/.exec(authority)
  if (!authorityMatch) return ''
  const [, hostname, port = ''] = authorityMatch
  if (!validHostname(hostname) || !validPort(port)) return ''
  return `https://${authority}`
}

export function resolveContentAsset(value, fallback = '') {
  const safeFallback = typeof fallback === 'string' ? fallback : ''
  if (typeof value !== 'string') return safeFallback
  if (/[\u0000-\u001f\u007f-\u009f]/.test(value)) return safeFallback
  const asset = value.trim()
  if (!asset || asset.startsWith('//')) return safeFallback

  if (/^https:\/\//i.test(asset)) {
    return httpsOrigin(asset) && isSafelyEncodedPath(asset) ? asset : safeFallback
  }

  if (isSafeLocalPath(asset, '/static/')) return asset
  if (!isSafeLocalPath(asset, '/assets/') && !isSafeLocalPath(asset, '/api/public/site-uploads/')) return safeFallback

  const origin = httpsOrigin(API_BASE)
  return origin ? `${origin}${asset}` : safeFallback
}
