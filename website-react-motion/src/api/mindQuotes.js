const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

// 拉取成长心语分组（含每组的轻量心语：id + 简短文案）
export async function getMindGroups() {
  const res = await fetch(`${API_BASE}/public/mind-groups`, {
    headers: { Accept: 'application/json' },
  })
  const body = await res.json().catch(() => ({}))
  if (!res.ok || body?.code !== 0) {
    throw new Error(body?.error || body?.message || '加载失败')
  }
  return body.data?.items || []
}

// 拉取单条心语的完整原文（详情页）
export async function getMindQuote(id) {
  const res = await fetch(`${API_BASE}/public/mind-quotes/${id}`, {
    headers: { Accept: 'application/json' },
  })
  const body = await res.json().catch(() => ({}))
  if (!res.ok || body?.code !== 0) {
    throw new Error(body?.error || body?.message || '加载失败')
  }
  return body.data
}
