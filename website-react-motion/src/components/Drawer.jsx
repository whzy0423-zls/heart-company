import { Link, useLocation } from 'react-router-dom'
import { DRAWER_LINKS } from './navData'

export default function Drawer({ open, onClose }) {
  const { pathname, hash } = useLocation()
  const current = pathname + hash

  const isActive = (to) => {
    if (to === '/') return pathname === '/' && !hash
    if (to.startsWith('/#')) return current === to
    if (to === '/stages') return pathname.startsWith('/stage')
    return pathname === to
  }

  return (
    <div className={open ? 'drawer open' : 'drawer'} onClick={onClose}>
      <div className="drawer__panel" onClick={(e) => e.stopPropagation()}>
        <button className="drawer__close" onClick={onClose}>×</button>
        {DRAWER_LINKS.map((it) => (
          <Link key={it.label} to={it.to} onClick={onClose}
                className={isActive(it.to) ? 'active' : undefined}>
            {it.label}
          </Link>
        ))}
      </div>
    </div>
  )
}
