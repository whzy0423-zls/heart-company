import { useEffect } from 'react'
import { useMotionPreferences } from './useMotionPreferences'

export function usePointerField() {
  const reducedMotion = useMotionPreferences()

  useEffect(() => {
    if (reducedMotion) return undefined
    const shell = document.querySelector('.motion-shell')
    if (!shell) return undefined
    const spotlightSections = [...document.querySelectorAll('[data-pointer-spotlight]')]
    const spotlightCleanups = []
    let frame = 0
    const onMove = (event) => {
      if (frame) return
      frame = requestAnimationFrame(() => {
        const x = (event.clientX / window.innerWidth) * 100
        const y = (event.clientY / window.innerHeight) * 100
        shell.style.setProperty('--pointer-x', `${x}%`)
        shell.style.setProperty('--pointer-y', `${y}%`)
        shell.style.setProperty('--pointer-x-num', String(x))
        shell.style.setProperty('--pointer-y-num', String(y))
        frame = 0
      })
    }

    spotlightSections.forEach((section) => {
      let spotlightFrame = 0
      let latestEvent
      const updateSpotlight = () => {
        const rect = section.getBoundingClientRect()
        const x = Math.min(Math.max(latestEvent.clientX - rect.left, 0), rect.width)
        const y = Math.min(Math.max(latestEvent.clientY - rect.top, 0), rect.height)
        section.style.setProperty('--spotlight-x', `${x}px`)
        section.style.setProperty('--spotlight-y', `${y}px`)
        spotlightFrame = 0
      }
      const onSpotlightMove = (event) => {
        latestEvent = event
        if (!spotlightFrame) spotlightFrame = requestAnimationFrame(updateSpotlight)
      }
      const onSpotlightEnter = (event) => {
        section.classList.add('is-spotlight-active')
        onSpotlightMove(event)
      }
      const onSpotlightLeave = () => {
        section.classList.remove('is-spotlight-active')
        section.style.setProperty('--spotlight-x', '68%')
        section.style.setProperty('--spotlight-y', '42%')
      }

      section.addEventListener('pointerenter', onSpotlightEnter, { passive: true })
      section.addEventListener('pointermove', onSpotlightMove, { passive: true })
      section.addEventListener('pointerleave', onSpotlightLeave, { passive: true })
      spotlightCleanups.push(() => {
        section.removeEventListener('pointerenter', onSpotlightEnter)
        section.removeEventListener('pointermove', onSpotlightMove)
        section.removeEventListener('pointerleave', onSpotlightLeave)
        if (spotlightFrame) cancelAnimationFrame(spotlightFrame)
      })
    })

    window.addEventListener('pointermove', onMove, { passive: true })
    return () => {
      window.removeEventListener('pointermove', onMove)
      if (frame) cancelAnimationFrame(frame)
      spotlightCleanups.forEach((cleanup) => cleanup())
    }
  }, [reducedMotion])
}
