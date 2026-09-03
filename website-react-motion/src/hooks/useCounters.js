import { useEffect } from 'react'

// 数字滚动：观察 [data-count]，进入视口时从 0 累加到目标值（沿用原 script.js 逻辑）
export function useCounters(dep) {
  useEffect(() => {
    const counters = document.querySelectorAll('[data-count]')
    if (!counters.length) return
    const cio = new IntersectionObserver(
      (es) =>
        es.forEach((en) => {
          if (!en.isIntersecting) return
          const el = en.target
          const target = +el.dataset.count
          const suf = el.dataset.suffix || ''
          let n = 0
          const step = target / 40
          const t = setInterval(() => {
            n += step
            if (n >= target) {
              n = target
              clearInterval(t)
            }
            el.textContent = Math.round(n) + suf
          }, 22)
          cio.unobserve(el)
        }),
      { threshold: 0.5 }
    )
    counters.forEach((c) => cio.observe(c))
    return () => cio.disconnect()
  }, [dep])
}
