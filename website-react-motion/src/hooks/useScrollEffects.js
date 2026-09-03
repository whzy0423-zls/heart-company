import { useEffect } from 'react'

// 滚动进度条 + 极光光晕视差（沿用原 script.js onScroll 逻辑）
export function useScrollEffects() {
  useEffect(() => {
    const prog = document.querySelector('.scroll-prog')
    const orbs = document.querySelectorAll('.fx-bg .orb')
    function onScroll() {
      const h = document.documentElement
      const sc = h.scrollTop / (h.scrollHeight - h.clientHeight || 1)
      if (prog) prog.style.width = sc * 100 + '%'
      orbs.forEach((o, i) => {
        o.style.transform = `translateY(${h.scrollTop * (0.04 + i * 0.025)}px)`
      })
    }
    const handler = () => requestAnimationFrame(onScroll)
    window.addEventListener('scroll', handler, { passive: true })
    onScroll()
    return () => window.removeEventListener('scroll', handler)
  }, [])
}

// 卡片鼠标光斑（全局 pointermove，更新 --mx/--my）
export function useCardSpotlight() {
  useEffect(() => {
    function onMove(e) {
      const card = e.target.closest && e.target.closest('.card')
      if (!card) return
      const r = card.getBoundingClientRect()
      card.style.setProperty('--mx', e.clientX - r.left + 'px')
      card.style.setProperty('--my', e.clientY - r.top + 'px')
    }
    document.addEventListener('pointermove', onMove)
    return () => document.removeEventListener('pointermove', onMove)
  }, [])
}
