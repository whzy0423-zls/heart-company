const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'
const VISITOR_ID_KEY = 'nine-xing-visitor-id'
const LANDING_PAGE_KEY = 'nine-xing-landing-page'
const LAST_GAME_RESULT_KEY = 'nine-xing-last-game-result'

export function getLandingPage() {
  try {
    const cached = sessionStorage.getItem(LANDING_PAGE_KEY)
    if (cached) return cached
    const value = window.location.href
    sessionStorage.setItem(LANDING_PAGE_KEY, value)
    return value
  } catch {
    return typeof window !== 'undefined' ? window.location.href : ''
  }
}

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

export function getAttribution(extra = {}) {
  const params = new URLSearchParams(window.location.search)
  return {
    landingPage: getLandingPage(),
    referrer: document.referrer,
    sourcePath: window.location.pathname + window.location.search + window.location.hash,
    utmCampaign: params.get('utm_campaign') || '',
    utmContent: params.get('utm_content') || '',
    utmMedium: params.get('utm_medium') || '',
    utmSource: params.get('utm_source') || '',
    utmTerm: params.get('utm_term') || '',
    visitorId: getVisitorId(),
    ...extra,
  }
}

export function getLastGameResult() {
  try {
    const raw = sessionStorage.getItem(LAST_GAME_RESULT_KEY)
    return raw ? JSON.parse(raw) : null
  } catch {
    return null
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

export async function trackGameResult(payload = {}) {
  const body = JSON.stringify({
    visitorId: getVisitorId(),
    ...payload,
  })

  const res = await fetch(`${API_BASE}/public/game-results`, {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    body,
    keepalive: true,
  }).catch(() => null)
  if (!res) return null
  const data = await res.json().catch(() => ({}))
  if (res.ok && data?.code === 0 && data.data) {
    try {
      sessionStorage.setItem(LAST_GAME_RESULT_KEY, JSON.stringify(data.data))
    } catch {}
    return data.data
  }
  return null
}
