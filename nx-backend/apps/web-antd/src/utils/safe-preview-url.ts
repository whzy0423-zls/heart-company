const CONTROL_RE = /[\u0000-\u001F\u007F]/;
const LOCAL_HTTP_HOSTS = new Set(['127.0.0.1', '::1', 'localhost']);

function isLocalHTTPPreviewHost(hostname: string) {
  const normalized = hostname.trim().toLowerCase().replace(/\.$/, '');
  return LOCAL_HTTP_HOSTS.has(normalized);
}

export function isSafePreviewURL(value?: null | string) {
  const source = value?.trim() || '';
  if (!source || CONTROL_RE.test(source) || source.startsWith('//')) {
    return false;
  }

  try {
    const base =
      typeof window === 'undefined' ? 'https://preview.local' : location.origin;
    const url = new URL(source, base);
    if (url.protocol === 'https:' || url.protocol === 'blob:') return true;
    return url.protocol === 'http:' && isLocalHTTPPreviewHost(url.hostname);
  } catch {
    return false;
  }
}
