export function resolveApiMediaUrl(path, apiBase = '/api') {
  if (!path) return ''
  const value = String(path).trim()
  if (!value) return ''
  if (/^\/\//.test(value)) return ''
  if (/^[a-z][a-z\d+.-]*:/i.test(value)) {
    return /^https:\/\//i.test(value) ? value : ''
  }
  if (/^https?:\/\//i.test(apiBase)) {
    try {
      const apiOrigin = new URL(apiBase).origin
      return new URL(value, `${apiOrigin}/`).toString()
    } catch {
      return value
    }
  }
  return value
}
