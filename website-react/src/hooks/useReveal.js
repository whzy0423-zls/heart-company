import { useEffect } from 'react'

// 滚动揭示：观察页面内所有 .reveal 元素，进入视口时加 .in
// 懒加载页面的 DOM 会在路由切换后才挂载，故用 MutationObserver 持续捕捉
// 新出现的 .reveal 元素，并加安全兜底，避免出现「内容空白」。
export function useReveal(dep) {
  useEffect(() => {
    let counter = 0
    const io = new IntersectionObserver(
      (entries) => {
        entries.forEach((en) => {
          if (en.isIntersecting) {
            en.target.classList.add('in')
            io.unobserve(en.target)
          }
        })
      },
      { threshold: 0.12 }
    )

    // 扫描并观察尚未处理的 .reveal 元素（可重复调用）
    const scan = () => {
      document.querySelectorAll('.reveal:not(.in)').forEach((el) => {
        if (el.dataset.revealObserved) return
        el.dataset.revealObserved = '1'
        el.style.transitionDelay = (counter++ % 4) * 80 + 'ms'
        io.observe(el)
      })
    }

    // 首帧扫描（已渲染内容，如首页）
    const id = requestAnimationFrame(scan)

    // 懒加载页面后续挂载时继续扫描
    const mo = new MutationObserver(scan)
    mo.observe(document.body, { childList: true, subtree: true })

    // 安全兜底：若 600ms 后仍有元素既未进入视口也未观察成功，
    // 直接显示，杜绝整段内容停留在 opacity:0 的空白状态。
    const fallback = setTimeout(() => {
      document.querySelectorAll('.reveal:not(.in)').forEach((el) => {
        const r = el.getBoundingClientRect()
        if (r.top < window.innerHeight && r.bottom > 0) el.classList.add('in')
      })
    }, 600)

    return () => {
      cancelAnimationFrame(id)
      clearTimeout(fallback)
      mo.disconnect()
      io.disconnect()
    }
  }, [dep])
}
