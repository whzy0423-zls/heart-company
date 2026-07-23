import { API_BASE } from '../config'

const DEFAULT_INTERVAL = 4000
const MIN_INTERVAL = 2000
const MAX_INTERVAL = 10000

function resolveImageUrl(image, apiBase) {
  const value = image.trim()
  if (!value || /^https?:\/\//i.test(value) || !value.startsWith('/api/')) return value

  try {
    return new URL(value, apiBase).origin + value
  } catch {
    return value
  }
}

export function filterFailedCarouselItems(carousel, failedImages = new Set()) {
  const items = Array.isArray(carousel?.items)
    ? carousel.items.filter((item) => !failedImages?.has?.(item?.image))
    : []

  return { ...carousel, items }
}

export function normalizeHomeCarousel(config, options = {}) {
  try {
    const carousel = config?.home?.miniappCarousel
    if (!carousel || typeof carousel !== 'object') {
      return { autoplay: true, interval: DEFAULT_INTERVAL, items: [] }
    }

    const interval = Number.isFinite(carousel.interval)
      ? Math.min(MAX_INTERVAL, Math.max(MIN_INTERVAL, carousel.interval))
      : DEFAULT_INTERVAL
    const apiBase = options && typeof options === 'object' && options.apiBase ? options.apiBase : API_BASE
    const seenImages = new Set()
    const items = Array.isArray(carousel.items)
      ? carousel.items
        .filter((item) => item && typeof item === 'object' && item.enabled !== false && typeof item.image === 'string' && item.image.trim())
        .map((item) => ({ image: resolveImageUrl(item.image, apiBase) }))
        .filter((item) => {
          if (seenImages.has(item.image)) return false
          seenImages.add(item.image)
          return true
        })
      : []

    return { autoplay: carousel.autoplay !== false, interval, items }
  } catch {
    return { autoplay: true, interval: DEFAULT_INTERVAL, items: [] }
  }
}
