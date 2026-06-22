// 阅读 H5 的公开接口封装。后端响应结构为 { code, data, error, message }。
const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

async function getJSON(path, params) {
  const url = new URL(`${API_BASE}${path}`, window.location.origin)
  if (params) {
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== null && value !== '') {
        url.searchParams.set(key, value)
      }
    })
  }
  const res = await fetch(url.toString().replace(window.location.origin, ''), {
    headers: { Accept: 'application/json' },
  })
  if (!res.ok) {
    let message = `请求失败 (${res.status})`
    try {
      const body = await res.json()
      message = body?.message || body?.error || message
    } catch {
      // ignore parse error
    }
    throw new Error(message)
  }
  const body = await res.json()
  return body?.data ?? body
}

// 文章列表（已发布）。
export function fetchArticles({ keyword, category, page = 1, pageSize = 20 } = {}) {
  return getJSON('/public/articles', { keyword, category, page, pageSize })
}

// 文章详情（含正文，会自增阅读量）。
export function fetchArticle(id) {
  return getJSON(`/public/articles/${id}`)
}

// 分类列表，用于顶部筛选。
export function fetchCategories() {
  return getJSON('/public/article-categories')
}

// 把后端返回的 /api/... 资源路径解析为可直接访问的地址。
// 当 API_BASE 是绝对地址时，用其源站拼接；否则保持同源相对路径。
export function resolveMediaUrl(path) {
  if (!path) return ''
  if (/^https?:\/\//i.test(path)) return path
  if (/^https?:\/\//i.test(API_BASE)) {
    try {
      return new URL(path, API_BASE).toString()
    } catch {
      return path
    }
  }
  return path
}
