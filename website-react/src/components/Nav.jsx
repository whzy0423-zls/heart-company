import { Link, useLocation } from 'react-router-dom'
import { MAIN_LINKS, isActive } from './navData'
import siteConfig from '../data/siteConfig'

export default function Nav({ onOpenDrawer }) {
  const { pathname } = useLocation()
  return (
    <header className="nav">
      <div className="wrap nav__inner">
        <Link className="brand" to="/">
          <img className="logo" src={siteConfig.site.logo} alt={siteConfig.site.brandName} />
          <span>{siteConfig.site.brandName}</span>
        </Link>
        <nav className="menu">
          {MAIN_LINKS.map((it) => (
            <Link key={it.label} to={it.to} className={isActive(it, pathname) ? 'active' : undefined}>
              {it.label}
            </Link>
          ))}
        </nav>
        <button className="nav__toggle" onClick={onOpenDrawer} aria-label="打开栏目菜单">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor"
               strokeWidth="2" strokeLinecap="round">
            <path d="M4 7h16M4 12h16M4 17h16" />
          </svg>
          <span>栏目</span>
        </button>
      </div>
    </header>
  )
}
