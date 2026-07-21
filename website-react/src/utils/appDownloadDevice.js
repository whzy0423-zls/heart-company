const MIB = 1024 * 1024

function readNavigatorSnapshot() {
  if (typeof navigator === 'undefined') {
    return { userAgent: '', platform: '', maxTouchPoints: 0 }
  }

  return {
    userAgent: navigator.userAgent,
    platform: navigator.platform,
    maxTouchPoints: navigator.maxTouchPoints,
  }
}

export function detectAppDownloadDevice(input) {
  const source = input ?? readNavigatorSnapshot()
  const userAgent = typeof source.userAgent === 'string' ? source.userAgent : ''
  const platform = typeof source.platform === 'string' ? source.platform : ''
  const maxTouchPoints = Number(source.maxTouchPoints) || 0

  if (/android/i.test(userAgent)) return 'android'
  if (/(iphone|ipad|ipod)/i.test(userAgent)) return 'ios'
  if (platform === 'MacIntel' && maxTouchPoints > 1) return 'ios'
  return 'desktop'
}

export function formatFileSizeMiB(bytes) {
  if (typeof bytes !== 'number' || !Number.isFinite(bytes) || bytes < 0) return '—'
  return `${(bytes / MIB).toFixed(1)} MiB`
}

export function formatLocalDate(value) {
  const date = new Date(value)
  if (!value || Number.isNaN(date.getTime())) return '—'

  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
}

export function formatSHA256(value) {
  if (typeof value !== 'string' || !value.trim()) return '—'
  return value.trim().replace(/\s+/g, '').match(/.{1,8}/g)?.join(' ') || '—'
}
