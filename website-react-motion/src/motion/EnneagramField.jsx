import { useEffect, useRef } from 'react'
import { useMotionPreferences } from './useMotionPreferences'

const DOTS = Array.from({ length: 42 }, (_, index) => ({
  x: (index * 37) % 100,
  y: (index * 61) % 100,
  r: 1 + (index % 3),
  d: (index % 8) * -.4,
}))

export default function EnneagramField() {
  const canvasRef = useRef(null)
  const reducedMotion = useMotionPreferences()

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return undefined
    const context = canvas.getContext('2d')
    const parent = canvas.parentElement
    let frame = 0
    let width = 0
    let height = 0
    const resize = () => {
      const rect = parent.getBoundingClientRect()
      const ratio = Math.min(window.devicePixelRatio || 1, 2)
      width = rect.width
      height = rect.height
      canvas.width = width * ratio
      canvas.height = height * ratio
      canvas.style.width = `${width}px`
      canvas.style.height = `${height}px`
      context.setTransform(ratio, 0, 0, ratio, 0, 0)
    }
    const draw = (time = 0) => {
      context.clearRect(0, 0, width, height)
      const pulse = reducedMotion ? 0 : Math.sin(time / 1300) * 4
      DOTS.forEach((dot, index) => {
        const x = width * dot.x / 100 + Math.sin(time / 1900 + dot.d) * 12
        const y = height * dot.y / 100 + Math.cos(time / 2200 + dot.d) * 10
        const alpha = .12 + ((index % 5) * .025)
        context.beginPath()
        context.fillStyle = index % 9 === 0 ? `rgba(217,83,82,${alpha + .1})` : `rgba(40,107,81,${alpha})`
        context.arc(x, y, dot.r + pulse / 18, 0, Math.PI * 2)
        context.fill()
      })
      if (!reducedMotion) frame = requestAnimationFrame(draw)
    }
    resize()
    draw()
    window.addEventListener('resize', resize)
    return () => { window.removeEventListener('resize', resize); cancelAnimationFrame(frame) }
  }, [reducedMotion])

  return <canvas className="enneagram-field" ref={canvasRef} aria-hidden="true" />
}
