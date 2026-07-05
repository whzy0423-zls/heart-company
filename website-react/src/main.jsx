import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import './index.css'
import App from './App.jsx'
import { hydrateSiteConfig } from './data/siteConfig'

// 后台公开站点配置接口地址（构建时注入，默认走同源 /api）。
const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'
const SITE_CONFIG_TIMEOUT_MS = 3500

const root = createRoot(document.getElementById('root'))

function renderApp() {
  root.render(
    <StrictMode>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </StrictMode>,
  )
}

// 首屏先使用构建时内置配置立即渲染；后台配置慢/失败不再阻塞官网加载。
async function hydrateSiteConfigFromBackend() {
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), SITE_CONFIG_TIMEOUT_MS)

  try {
    const res = await fetch(`${API_BASE}/public/site-config`, {
      headers: { Accept: 'application/json' },
      signal: controller.signal,
    })
    if (res.ok) {
      const body = await res.json()
      // 后端响应结构为 { code, data, error, message }
      hydrateSiteConfig(body?.data ?? body)
      renderApp()
    }
  } catch (err) {
    if (err?.name !== 'AbortError') {
      console.warn('[site-config] 拉取失败，使用内置默认配置：', err)
    }
  } finally {
    window.clearTimeout(timeout)
  }
}

renderApp()
hydrateSiteConfigFromBackend()
