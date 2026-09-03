import siteConfig from '../data/siteConfig'

export const MAIN_LINKS = siteConfig.navigation.main
export const DRAWER_LINKS = siteConfig.navigation.drawer
export const TAB_LINKS = siteConfig.navigation.tabs

// 判断主导航某项是否高亮
export function isActive(item, pathname, hash = '') {
  const current = pathname + hash
  if (item.to === '/') return pathname === '/' && !hash
  if (item.type === 'hash') return current === item.to
  if (item.to === '/stages') return pathname.startsWith('/stage') // stages + stage1/2/3
  if (item.to === '/courses') return pathname === '/courses' || current === '/#courses'
  return pathname === item.to
}
