const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'
const VISITOR_ID_KEY = 'nine-xing-visitor-id'

export function getVisitorId() {
  try {
    const cached = localStorage.getItem(VISITOR_ID_KEY)
    if (cached) return cached
    const id = crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}`
    localStorage.setItem(VISITOR_ID_KEY, id)
    return id
  } catch {
    return ''
  }
}

export function trackSiteVisit(payload = {}) {
  const body = JSON.stringify({
    path: window.location.pathname + window.location.search + window.location.hash,
    referrer: document.referrer,
    title: document.title,
    visitorId: getVisitorId(),
    ...payload,
  })

  if (navigator.sendBeacon) {
    const blob = new Blob([body], { type: 'application/json' })
    if (navigator.sendBeacon(`${API_BASE}/public/site-visits`, blob)) {
      return
    }
  }

  fetch(`${API_BASE}/public/site-visits`, {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    body,
    keepalive: true,
  }).catch(() => {})
}

export function trackGameResult(payload = {}) {
  const body = JSON.stringify({
    visitorId: getVisitorId(),
    ...payload,
  })

  fetch(`${API_BASE}/public/game-results`, {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    body,
    keepalive: true,
  }).catch(() => {})
}
