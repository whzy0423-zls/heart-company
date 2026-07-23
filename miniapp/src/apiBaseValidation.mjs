function cleanBaseUrl(value) {
  return String(value || '').trim().replace(/\/+$/, '')
}

function parsePort(value) {
  if (!value) return true
  if (!/^:\d+$/.test(value)) return false
  return Number(value.slice(1)) <= 65535
}

function parseIPv4Part(value) {
  let digits = value
  let radix = 10

  if (/^0x/i.test(digits)) {
    digits = digits.slice(2)
    radix = 16
    if (!/^[0-9a-f]+$/i.test(digits)) return null
  } else if (digits.length > 1 && digits.startsWith('0')) {
    digits = digits.slice(1) || '0'
    radix = 8
    if (!/^[0-7]+$/.test(digits)) return null
  } else if (!/^\d+$/.test(digits)) {
    return null
  }

  const parsed = Number.parseInt(digits, radix)
  return Number.isSafeInteger(parsed) ? parsed : null
}

function parseIPv4(hostname) {
  const rawParts = hostname.split('.')
  if (rawParts.length > 4 || rawParts.some((part) => !part)) return { matched: false }
  const numericCandidate = rawParts.every((part) => /^\d+$/.test(part) || /^0x/i.test(part))
  if (!numericCandidate) return { matched: false }

  const parts = rawParts.map(parseIPv4Part)
  if (parts.some((part) => part === null)) return { matched: true, valid: false }

  if (parts.slice(0, -1).some((part) => part > 255)) return { matched: true, valid: false }
  const lastLimit = 256 ** (5 - parts.length)
  const lastPart = parts[parts.length - 1]
  if (lastPart >= lastLimit) return { matched: true, valid: false }

  let numeric = lastPart
  for (let index = 0; index < parts.length - 1; index += 1) {
    numeric += parts[index] * 256 ** (3 - index)
  }

  const octets = [
    Math.floor(numeric / 256 ** 3) % 256,
    Math.floor(numeric / 256 ** 2) % 256,
    Math.floor(numeric / 256) % 256,
    numeric % 256,
  ]
  return { matched: true, valid: true, value: octets.join('.') }
}

function parseStrictDottedIPv4(value) {
  const parts = value.split('.')
  if (parts.length !== 4 || parts.some((part) => !/^\d{1,3}$/.test(part))) return null
  const octets = parts.map(Number)
  return octets.some((part) => part > 255) ? null : octets
}

function parseIPv6(value) {
  let address = value.toLowerCase()
  if (address.includes('.')) {
    const lastColon = address.lastIndexOf(':')
    if (lastColon < 0) return null
    const octets = parseStrictDottedIPv4(address.slice(lastColon + 1))
    if (!octets) return null
    const high = (octets[0] << 8) | octets[1]
    const low = (octets[2] << 8) | octets[3]
    address = `${address.slice(0, lastColon)}:${high.toString(16)}:${low.toString(16)}`
  }

  if ((address.match(/::/g) || []).length > 1) return null
  const hasCompression = address.includes('::')
  const [left = '', right = ''] = address.split('::')
  const leftParts = left ? left.split(':') : []
  const rightParts = right ? right.split(':') : []
  const allParts = [...leftParts, ...rightParts]
  if (allParts.some((part) => !/^[0-9a-f]{1,4}$/.test(part))) return null

  const missing = 8 - allParts.length
  if ((hasCompression && missing < 1) || (!hasCompression && missing !== 0)) return null

  return [
    ...leftParts.map((part) => Number.parseInt(part, 16)),
    ...Array(hasCompression ? missing : 0).fill(0),
    ...rightParts.map((part) => Number.parseInt(part, 16)),
  ]
}

function parseHttpsHost(apiBase) {
  if (/\s/.test(apiBase)) return null
  const rest = apiBase.slice('https://'.length)
  const boundary = rest.search(/[/?#]/)
  const authority = boundary < 0 ? rest : rest.slice(0, boundary)
  const hostAndPort = authority.slice(authority.lastIndexOf('@') + 1)
  if (!hostAndPort) return null

  if (hostAndPort.startsWith('[')) {
    const closingBracket = hostAndPort.indexOf(']')
    if (closingBracket < 0 || !parsePort(hostAndPort.slice(closingBracket + 1))) return null
    const segments = parseIPv6(hostAndPort.slice(1, closingBracket))
    return segments ? { type: 'ipv6', segments } : null
  }

  const colon = hostAndPort.lastIndexOf(':')
  if (colon !== hostAndPort.indexOf(':')) return null
  const hostnameWithDot = colon < 0 ? hostAndPort : hostAndPort.slice(0, colon)
  if (colon >= 0 && !parsePort(hostAndPort.slice(colon))) return null

  const hostname = hostnameWithDot.toLowerCase().replace(/\.+$/, '')
  if (!hostname || !/^[a-z0-9.-]+$/.test(hostname)) return null

  const ipv4 = parseIPv4(hostname)
  if (ipv4.matched) return ipv4.valid ? { type: 'ipv4', hostname: ipv4.value } : null

  const labels = hostname.split('.')
  if (labels.some((label) => !label || label.startsWith('-') || label.endsWith('-'))) return null
  return { type: 'domain', hostname }
}

function isPrivateIPv4(value) {
  const parts = value.split('.').map(Number)
  return (
    parts[0] === 10 ||
    parts[0] === 127 ||
    (parts[0] === 172 && parts[1] >= 16 && parts[1] <= 31) ||
    (parts[0] === 192 && parts[1] === 168) ||
    (parts[0] === 169 && parts[1] === 254) ||
    parts[0] === 0
  )
}

function isPrivateIPv6(segments) {
  const isUnspecified = segments.every((part) => part === 0)
  const isLoopback = segments.slice(0, -1).every((part) => part === 0) && segments[7] === 1
  if (isUnspecified || isLoopback) return true

  const isMappedIPv4 = segments.slice(0, 5).every((part) => part === 0) && segments[5] === 0xffff
  if (isMappedIPv4) {
    const mappedIPv4 = [
      segments[6] >> 8,
      segments[6] & 0xff,
      segments[7] >> 8,
      segments[7] & 0xff,
    ].join('.')
    return isPrivateIPv4(mappedIPv4)
  }

  const isUniqueLocal = (segments[0] & 0xfe00) === 0xfc00
  const isLinkLocal = (segments[0] & 0xffc0) === 0xfe80
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

function isBlockedHost(host) {
  if (host.type === 'ipv4') return isPrivateIPv4(host.hostname)
  if (host.type === 'ipv6') return isPrivateIPv6(host.segments)

  return (
    host.hostname === 'localhost' ||
    host.hostname.endsWith('.localhost') ||
    host.hostname.endsWith('.local') ||
    hasBlockedDomainSuffix(host.hostname, 'example') ||
    hasBlockedDomainSuffix(host.hostname, 'yourdomain')
  )
}

export function validateProductionApiBase(value) {
  const apiBase = cleanBaseUrl(value)
  if (!apiBase) return { ok: false, reason: 'required' }
  if (!apiBase.startsWith('https://')) return { ok: false, reason: 'https' }

  const host = parseHttpsHost(apiBase)
  if (!host) return { ok: false, reason: 'invalid' }
  if (isBlockedHost(host)) return { ok: false, reason: 'blocked' }

  return { ok: true, value: apiBase }
}
