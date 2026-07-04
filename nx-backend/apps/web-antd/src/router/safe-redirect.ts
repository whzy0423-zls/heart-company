export function safeDecodeURIComponent(value: unknown, fallback: string) {
  const raw = Array.isArray(value) ? value[0] : value;
  if (typeof raw !== 'string' || raw.length === 0) {
    return fallback;
  }
  try {
    return decodeURIComponent(raw);
  } catch {
    return fallback;
  }
}
