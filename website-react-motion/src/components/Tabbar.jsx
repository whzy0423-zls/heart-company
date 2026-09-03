import { Link, useLocation } from 'react-router-dom'
import { TAB_LINKS } from './navData'

const ICONS = {
  home: 'M3 11.5 12 4l9 7.5M5.5 10v9.5a1 1 0 0 0 1 1H10v-5h4v5h3.5a1 1 0 0 0 1-1V10',
  user: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8ZM5 20c.7-3.5 3.6-5.5 7-5.5s6.3 2 7 5.5',
  layers: 'M12 3 3 8l9 5 9-5-9-5ZM4 12.5 12 17l8-4.5M4 16.5 12 21l8-4.5',
  play: 'M9 7.5v9l7-4.5-7-4.5Z',
  edit: 'M4 20h4l10-10a2 2 0 0 0-3-3L5 17v3ZM13.5 6.5l3 3',
}

function Icon({ name }) {
  return (
    <svg viewBox="0 0 24 24" width="22" height="22" fill="none"
         stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
      <path d={ICONS[name]} />
    </svg>
  )
}

export default function Tabbar() {
  const { pathname, hash } = useLocation()
  return (
    <nav className="tabbar" aria-label="底部导航">
      {TAB_LINKS.map((it) => {
        let active = false
        if (it.match?.startsWith('#')) {
          active = pathname === '/' && hash === it.match
        } else if (it.match === '/') {
          active = pathname === '/' && !hash
        } else if (it.match) {
          active = pathname.startsWith(it.match)
        }
        return (
          <Link key={it.label} to={it.to} className={active ? 'active' : undefined}>
            <span className="tabbar__icon"><Icon name={it.icon} /></span>
            <span className="tabbar__label">{it.label}</span>
          </Link>
        )
      })}
    </nav>
  )
}
