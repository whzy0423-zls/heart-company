import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { HashRouter } from 'react-router-dom'
import App from './App.jsx'
import './styles/index.css'

function migrateLegacyBrowserArticlePath() {
  const { hash, pathname, search } = window.location
  if (hash || !/^\/article\/[^/]+\/?$/.test(pathname)) return
  window.history.replaceState(null, '', `/#${pathname.replace(/\/$/, '')}${search || ''}`)
}

migrateLegacyBrowserArticlePath()

createRoot(document.getElementById('root')).render(
  <StrictMode>
    <HashRouter>
      <App />
    </HashRouter>
  </StrictMode>,
)
