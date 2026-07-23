function cleanBaseUrl(value) {
  return String(value || '').trim().replace(/\/+$/, '')
}

function normalizeHostname(value) {
  return value.toLowerCase().replace(/^\[|\]$/g, '').replace(/\.+$/, '')
}

function isPrivateIPv4(value) {
  const parts = value.split('.').map((part) => Number(part))
  if (parts.length !== 4 || parts.some((part) => !Number.isInteger(part) || part < 0 || part > 255)) {
    return false
  }

  return (
    parts[0] === 10 ||
    parts[0] === 127 ||
    (parts[0] === 172 && parts[1] >= 16 && parts[1] <= 31) ||
    (parts[0] === 192 && parts[1] === 168) ||
    (parts[0] === 169 && parts[1] === 254) ||
    parts[0] === 0
  )
}

function isPrivateIPv6(value) {
  if (value === '::1') return true

  const firstHextet = Number.parseInt(value.split(':', 1)[0], 16)
  if (!Number.isInteger(firstHextet)) return false

  const isUniqueLocal = (firstHextet & 0xfe00) === 0xfc00
  const isLinkLocal = (firstHextet & 0xffc0) === 0xfe80
  return isUniqueLocal || isLinkLocal
}

function hasBlockedDomainSuffix(hostname, secondLevelLabel) {
  const labels = hostname.split('.')
  return (
    labels.length >= 2 &&
    labels[labels.length - 2] === secondLevelLabel &&
    labels[labels.length - 1] === 'com'
  )
}

function isBlockedHostname(hostname) {
  return (
    hostname === 'localhost' ||
    hostname.endsWith('.localhost') ||
    hostname.endsWith('.local') ||
    isPrivateIPv4(hostname) ||
    isPrivateIPv6(hostname) ||
    hasBlockedDomainSuffix(hostname, 'example') ||
    hasBlockedDomainSuffix(hostname, 'yourdomain')
  )
}

export function validateProductionApiBase(value) {
  const apiBase = cleanBaseUrl(value)
  if (!apiBase) return { ok: false, reason: 'required' }
  if (!apiBase.startsWith('https://')) return { ok: false, reason: 'https' }

  let parsed
  try {
    parsed = new URL(apiBase)
  } catch {
    return { ok: false, reason: 'invalid' }
  }

  const hostname = normalizeHostname(parsed.hostname)
  if (!hostname) return { ok: false, reason: 'invalid' }
  if (isBlockedHostname(hostname)) return { ok: false, reason: 'blocked' }

  return { ok: true, value: apiBase }
}
