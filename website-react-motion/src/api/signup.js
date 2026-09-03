import { getAttribution, getLastGameResult } from './analytics'

const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

export async function submitSignup(payload) {
  const gameResult = getLastGameResult()
  const res = await fetch(`${API_BASE}/public/signups`, {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      ...payload,
      ...getAttribution(),
      gameResultId: gameResult?.id || '',
    }),
  })
  const body = await res.json().catch(() => ({}))
  if (!res.ok || body?.code !== 0) {
    throw new Error(body?.error || body?.message || '提交失败')
  }
  return body.data
}
