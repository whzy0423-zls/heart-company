import { useEffect, useState } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import FxBackground from './FxBackground'
import ScrollProgress from './ScrollProgress'
import Nav from './Nav'
import Drawer from './Drawer'
import Tabbar from './Tabbar'
import Music from './Music'
import Footer from './Footer'
import Lightbox from './Lightbox'
import { useScrollEffects, useCardSpotlight } from '../hooks/useScrollEffects'
import { useReveal } from '../hooks/useReveal'
import { useCounters } from '../hooks/useCounters'

export default function Layout() {
  const [drawerOpen, setDrawerOpen] = useState(false)
  const location = useLocation()
  const key = location.pathname

  // 全局动效（挂载一次）
  useScrollEffects()
  useCardSpotlight()
  // 路由切换后重新扫描揭示动效 / 数字滚动
  useReveal(key)
  useCounters(key)

  // 路由切换：有 hash 滚到对应区块，否则回到顶部
  useEffect(() => {
    setDrawerOpen(false)
    if (location.hash) {
      const id = location.hash.slice(1)
      // 等懒加载页面渲染完成
      const t = setTimeout(() => {
        const el = document.getElementById(id)
        if (el) el.scrollIntoView({ behavior: 'smooth' })
      }, 60)
      return () => clearTimeout(t)
    }
    window.scrollTo(0, 0)
  }, [location.pathname, location.hash])

  return (
    <>
      <FxBackground />
      <ScrollProgress />
      <Nav onOpenDrawer={() => setDrawerOpen(true)} />
      <Drawer open={drawerOpen} onClose={() => setDrawerOpen(false)} />
      <main>
        <Outlet />
      </main>
      <Footer />
      <Music />
      <Tabbar />
      <Lightbox />
    </>
  )
}
