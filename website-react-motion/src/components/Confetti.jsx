import { useEffect, useRef } from 'react'

// 轻量彩纸礼花：canvas 自绘，无第三方依赖。挂载即放一波，随后持续小量飘落。
export default function Confetti({ duration = 2600 }) {
  const ref = useRef(null)

  useEffect(() => {
    const canvas = ref.current
    const ctx = canvas.getContext('2d')
    let raf, running = true
    const DPR = Math.min(window.devicePixelRatio || 1, 2)
    const resize = () => {
      canvas.width = canvas.offsetWidth * DPR
      canvas.height = canvas.offsetHeight * DPR
    }
    resize()
    window.addEventListener('resize', resize)

    const colors = ['#e23a47', '#2b7fff', '#25b365', '#ffb020', '#ff5a6a', '#5aa0ff']
    const W = () => canvas.width
    const H = () => canvas.height
    const rand = (a, b) => a + Math.random() * (b - a)

    const make = (burst) => ({
      x: rand(0, W()),
      y: burst ? rand(-H() * 0.1, H() * 0.4) : rand(-40, -10),
      r: rand(5, 11) * DPR,
      c: colors[(Math.random() * colors.length) | 0],
      vx: rand(-1.2, 1.2) * DPR,
      vy: rand(2, 5) * DPR,
      rot: rand(0, Math.PI * 2),
      vr: rand(-0.2, 0.2),
      shape: Math.random() > 0.5 ? 'rect' : 'circ',
    })

    let parts = Array.from({ length: 140 }, () => make(true))
    const start = performance.now()

    const tick = (now) => {
      if (!running) return
      ctx.clearRect(0, 0, W(), H())
      const elapsed = now - start
      // 持续补充少量，直到接近结束
      if (elapsed < duration - 600 && parts.length < 180 && Math.random() > 0.6) {
        parts.push(make(false))
      }
      parts.forEach((p) => {
        p.x += p.vx; p.y += p.vy; p.vy += 0.03 * DPR; p.rot += p.vr
        ctx.save(); ctx.translate(p.x, p.y); ctx.rotate(p.rot); ctx.fillStyle = p.c
        if (p.shape === 'rect') ctx.fillRect(-p.r / 2, -p.r / 2, p.r, p.r * 0.6)
        else { ctx.beginPath(); ctx.arc(0, 0, p.r / 2, 0, Math.PI * 2); ctx.fill() }
        ctx.restore()
      })
      parts = parts.filter((p) => p.y < H() + 30)
      if (elapsed < duration || parts.length) raf = requestAnimationFrame(tick)
    }
    raf = requestAnimationFrame(tick)

    return () => { running = false; cancelAnimationFrame(raf); window.removeEventListener('resize', resize) }
  }, [duration])

  return <canvas ref={ref} className="confetti-canvas" aria-hidden="true" />
}
