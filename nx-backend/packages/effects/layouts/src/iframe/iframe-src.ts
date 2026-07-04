const CONTROL_CHARS = /[\u0000-\u001F\u007F]/;

function currentOrigin() {
  if (typeof window === 'undefined') {
    return 'http://localhost';
  }
  return window.location.origin;
}

export function safeIframeSrc(raw: unknown, base = currentOrigin()) {
  if (typeof raw !== 'string') return 'about:blank';

  const value = raw.trim();
  if (!value || CONTROL_CHARS.test(value) || value.startsWith('//')) {
    return 'about:blank';
  }

  try {
    const url = new URL(value, base);
    const origin = new URL(base).origin;
    if ((url.protocol === 'http:' || url.protocol === 'https:') && url.origin === origin) {
      return url.href;
    }
  } catch {
    return 'about:blank';
  }

  return 'about:blank';
}
