import { useCallback, useEffect, useState } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import FxBackground from './FxBackground'
import ScrollProgress from './ScrollProgress'
import Nav from './Nav'
import Drawer from './Drawer'
import Tabbar from './Tabbar'
import Music from './Music'
import Footer from './Footer'
import Lightbox from './Lightbox'
import CustomerService from './CustomerService'
import { useScrollEffects, useCardSpotlight } from '../hooks/useScrollEffects'
import { useReveal } from '../hooks/useReveal'
import { useCounters } from '../hooks/useCounters'

export default function Layout() {
  const [drawerOpen, setDrawerOpen] = useState(false)
  const location = useLocation()
  const key = location.pathname
  const isAppPage = location.pathname.replace(/\/+$/, '') === '/app'
  const closeDrawer = useCallback(() => setDrawerOpen(false), [])

  // 全局动效（挂载一次）
  useScrollEffects()
  useCardSpotlight()
  // 路由切换后重新扫描揭示动效 / 数字滚动
  useReveal(key)
  useCounters(key)

  // 路由切换：有 hash 滚到对应区块，否则回到顶部
  useEffect(() => {
    closeDrawer()
    let attempts = 0
    let timer = 0

    const moveFocus = () => {
      const target = location.hash
        ? document.getElementById(location.hash.slice(1))
        : document.getElementById('main-content')

      if (!target) {
        attempts += 1
        if (attempts < 40) timer = window.setTimeout(moveFocus, 50)
        return
      }

      if (location.hash) {
        const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
        target.scrollIntoView({ behavior: reduceMotion ? 'auto' : 'smooth', block: 'start' })
      } else {
        window.scrollTo(0, 0)
      }
      target.focus({ preventScroll: true })
    }

    timer = window.setTimeout(moveFocus, location.hash ? 60 : 0)
    return () => window.clearTimeout(timer)
  }, [closeDrawer, location.pathname, location.hash])

  return (
    <>
      <a className="skip-link" href="#main-content">跳到主要内容</a>
      <FxBackground />
      <ScrollProgress />
      <Nav drawerOpen={drawerOpen} onOpenDrawer={() => setDrawerOpen(true)} />
      <Drawer open={drawerOpen} onClose={closeDrawer} />
      <main id="main-content" tabIndex="-1">
        <Outlet />
      </main>
      <Footer />
      {!isAppPage && <Music />}
      {!isAppPage && <CustomerService />}
      {!isAppPage && <Tabbar />}
      <Lightbox />
    </>
  )
}
