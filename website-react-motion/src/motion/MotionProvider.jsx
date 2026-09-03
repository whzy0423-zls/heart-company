import { useEffect } from 'react'
import { gsap } from 'gsap'
import { ScrollTrigger } from 'gsap/ScrollTrigger'
import Lenis from 'lenis'
import { useMotionPreferences } from './useMotionPreferences'
import { usePointerField } from './usePointerField'

gsap.registerPlugin(ScrollTrigger)

export default function MotionProvider({ children }) {
  const reducedMotion = useMotionPreferences()
  usePointerField()

  useEffect(() => {
    document.documentElement.classList.toggle('motion-reduced', reducedMotion)
    const revealItems = [...document.querySelectorAll('[data-motion-reveal]')]
    const cleanups = []
    let lenis
    if (reducedMotion) {
      document.documentElement.classList.add('motion-reduced')
      revealItems.forEach((item) => item.classList.add('is-visible'))
      return undefined
    }

    lenis = new Lenis({ duration: 1.05, smoothWheel: true, syncTouch: false })
    const raf = (time) => {
      lenis.raf(time)
      frame = requestAnimationFrame(raf)
    }
    let frame = requestAnimationFrame(raf)
    const updateScroll = () => ScrollTrigger.update()
    lenis.on('scroll', updateScroll)

    revealItems.forEach((item, index) => {
      const tween = gsap.fromTo(item,
        { autoAlpha: 0, y: 44 },
        { autoAlpha: 1, y: 0, duration: .9, delay: Math.min(index * .035, .22), ease: 'power3.out', paused: true },
      )
      ScrollTrigger.create({
        trigger: item,
        start: 'top 84%',
        once: true,
        onEnter: () => { item.classList.add('is-visible'); tween.play() },
      })
      cleanups.push(() => tween.kill())
    })

    document.querySelectorAll('[data-motion-title]').forEach((title) => {
      const tween = gsap.fromTo(title.querySelectorAll('.motion-word'),
        { yPercent: 105, opacity: 0 },
        { yPercent: 0, opacity: 1, duration: .8, stagger: .045, ease: 'power4.out', paused: true },
      )
      ScrollTrigger.create({ trigger: title, start: 'top 82%', once: true, onEnter: () => tween.play() })
      cleanups.push(() => tween.kill())
    })

    document.querySelectorAll('[data-parallax]').forEach((element) => {
      const tween = gsap.to(element, { yPercent: -10, ease: 'none', scrollTrigger: { trigger: element, scrub: true } })
      cleanups.push(() => tween.kill())
    })

    document.querySelectorAll('[data-kinetic-track]').forEach((track) => {
      const tween = gsap.fromTo(track, { xPercent: 4 }, { xPercent: -20, ease: 'none', scrollTrigger: { trigger: track.parentElement, start: 'top bottom', end: 'bottom top', scrub: 1 } })
      cleanups.push(() => tween.kill())
    })

    document.querySelectorAll('[data-magnetic]').forEach((element) => {
      const onMove = (event) => {
        const rect = element.getBoundingClientRect()
        gsap.to(element, { x: (event.clientX - rect.left - rect.width / 2) * .16, y: (event.clientY - rect.top - rect.height / 2) * .16, duration: .35, ease: 'power3.out' })
      }
      const onLeave = () => gsap.to(element, { x: 0, y: 0, duration: .6, ease: 'elastic.out(1, .35)' })
      element.addEventListener('pointermove', onMove)
      element.addEventListener('pointerleave', onLeave)
      cleanups.push(() => { element.removeEventListener('pointermove', onMove); element.removeEventListener('pointerleave', onLeave) })
    })

    const hero = document.querySelector('[data-motion-section="hero"]')
    const shell = document.querySelector('.motion-shell')
    if (hero && shell) {
      const updateFloatingTools = () => {
        const threshold = hero.offsetTop + hero.offsetHeight - window.innerHeight * .76
        shell.classList.toggle('motion-fabs-visible', window.scrollY > threshold)
      }
      lenis.on('scroll', updateFloatingTools)
      window.addEventListener('resize', updateFloatingTools)
      updateFloatingTools()
      cleanups.push(() => {
        window.removeEventListener('resize', updateFloatingTools)
        shell.classList.remove('motion-fabs-visible')
      })
    }

    return () => {
      cancelAnimationFrame(frame)
      lenis?.destroy()
      cleanups.forEach((cleanup) => cleanup())
      ScrollTrigger.getAll().forEach((trigger) => trigger.kill())
    }
  }, [reducedMotion])

  return <div className="motion-shell">{children}</div>
}
