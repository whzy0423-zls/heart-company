import { useEffect, useRef } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { DRAWER_LINKS } from './navData'

const FOCUSABLE_SELECTOR = 'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])'

export default function Drawer({ open, onClose }) {
  const { pathname, hash } = useLocation()
  const current = pathname + hash
  const panelRef = useRef(null)
  const lastActiveElementRef = useRef(null)

  const isActive = (to) => {
    if (to === '/') return pathname === '/' && !hash
    if (to.startsWith('/#')) return current === to
    if (to === '/stages') return pathname.startsWith('/stage')
    return pathname === to
  }

  useEffect(() => {
    if (!open) return undefined

    const panel = panelRef.current
    const previousOverflow = document.body.style.overflow
    lastActiveElementRef.current = document.activeElement
    document.body.style.overflow = 'hidden'

    const getFocusableElements = () => Array.from(panel?.querySelectorAll(FOCUSABLE_SELECTOR) ?? [])
    const focusTimer = window.setTimeout(() => getFocusableElements()[0]?.focus(), 0)
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onClose()
        return
      }
      if (event.key !== 'Tab') return

      const focusableElements = getFocusableElements()
      if (focusableElements.length === 0) {
        event.preventDefault()
        return
      }

      const first = focusableElements[0]
      const last = focusableElements[focusableElements.length - 1]
      if (!panel?.contains(document.activeElement)) {
        event.preventDefault()
        first.focus()
      } else if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first.focus()
      }
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => {
      window.clearTimeout(focusTimer)
      document.removeEventListener('keydown', handleKeyDown)
      document.body.style.overflow = previousOverflow
      window.requestAnimationFrame(() => lastActiveElementRef.current?.focus())
    }
  }, [open, onClose])

  return (
    <div className={open ? 'drawer open' : 'drawer'} onClick={onClose} aria-hidden={!open}>
      <div
        className="drawer__panel"
        id="site-drawer"
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label="栏目菜单"
        onClick={(event) => event.stopPropagation()}
      >
        <button className="drawer__close" type="button" onClick={onClose} aria-label="关闭栏目菜单">×</button>
        {DRAWER_LINKS.map((it) => (
          <Link key={it.label} to={it.to} onClick={onClose}
                className={isActive(it.to) ? 'active' : undefined}
                aria-current={isActive(it.to) ? 'page' : undefined}>
            {it.label}
          </Link>
        ))}
      </div>
    </div>
  )
}
