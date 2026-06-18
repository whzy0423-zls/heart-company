import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import './index.css'
import { hydrateSiteConfig } from './data/siteConfig'

// 后台公开站点配置接口地址（构建时注入，默认走同源 /api）。
const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'

// 渲染前先拉取最新站点配置并 hydrate；失败则用构建时内置的兜底配置。
async function bootstrap() {
  try {
    const res = await fetch(`${API_BASE}/public/site-config`, {
      headers: { Accept: 'application/json' },
    })
    if (res.ok) {
      const body = await res.json()
      // 后端响应结构为 { code, data, error, message }
      hydrateSiteConfig(body?.data ?? body)
    }
  } catch (err) {
    console.warn('[site-config] 拉取失败，使用内置默认配置：', err)
  }

  // 动态 import：确保 App 及其依赖（navData/types/各组件）在 hydrate 之后才加载。
  const { default: App } = await import('./App.jsx')

  createRoot(document.getElementById('root')).render(
    <StrictMode>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </StrictMode>,
  )
}

bootstrap()
