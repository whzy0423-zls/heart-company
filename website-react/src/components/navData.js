import siteConfig from '../data/siteConfig'

export const MAIN_LINKS = siteConfig.navigation.main
export const DRAWER_LINKS = siteConfig.navigation.drawer
export const TAB_LINKS = siteConfig.navigation.tabs

// 判断主导航某项是否高亮
export function isActive(item, pathname) {
  if (item.type !== 'route') return false
  if (item.to === '/') return pathname === '/'
  if (item.to === '/stages') return pathname.startsWith('/stage') // stages + stage1/2/3
  return pathname === item.to
}
