import bundledConfig from '../../../shared/site-config.json'

// 站点配置：构建时内置一份默认值作兜底，运行时由 main.jsx 在渲染前
// 用后台公开接口拉取的最新配置原地填充（hydrate）。
// 所有消费方（navData.js / types.js / 各组件）只要在 hydrate 之后被加载，
// 读到的就是最新配置，无需改动它们。
const siteConfig = structuredClone(bundledConfig)

function isProtectedAppEntry(item) {
  return item?.label === '下载 App' && item?.to === '/app'
}

function ensureProtectedAppEntry(items, fallback) {
  if (!Array.isArray(items) || !fallback) return
  const index = items.findIndex((item) => item?.label === '下载 App')
  if (index >= 0) {
    items[index] = structuredClone(fallback)
    return
  }
  items.push(structuredClone(fallback))
}

function keepProtectedAppEntries(next) {
  const merged = structuredClone(next)
  if (!merged.navigation || typeof merged.navigation !== 'object' || Array.isArray(merged.navigation)) {
    merged.navigation = {}
  }
  const navigation = merged.navigation
  for (const key of ['main', 'drawer']) {
    if (!Array.isArray(navigation[key])) navigation[key] = []
    const fallback = bundledConfig.navigation[key].find(isProtectedAppEntry)
    ensureProtectedAppEntry(navigation[key], fallback)
  }

  if (!merged.home || typeof merged.home !== 'object' || Array.isArray(merged.home)) {
    merged.home = {}
  }
  merged.home.appDownload = structuredClone(bundledConfig.home.appDownload)
  if (!merged.home.hero || typeof merged.home.hero !== 'object' || Array.isArray(merged.home.hero)) {
    merged.home.hero = {}
  }
  if (!Array.isArray(merged.home.hero.actions)) merged.home.hero.actions = []
  const actions = merged?.home?.hero?.actions
  const fallback = bundledConfig.home.hero.actions.find(isProtectedAppEntry)
  ensureProtectedAppEntry(actions, fallback)
  return merged
}

function replaceInPlace(target, source) {
  if (!source || typeof source !== 'object') return source

  if (Array.isArray(source)) {
    if (!Array.isArray(target)) {
      return structuredClone(source)
    }
    target.splice(0, target.length, ...source.map((item, index) => replaceInPlace(target[index], item)))
    return target
  }

  if (!target || typeof target !== 'object' || Array.isArray(target)) {
    return structuredClone(source)
  }

  for (const key of Object.keys(target)) {
    if (!(key in source)) delete target[key]
  }

  for (const key of Object.keys(source)) {
    target[key] = replaceInPlace(target[key], source[key])
  }

  return target
}

// 用后台返回的数据原地覆盖，保留已被 navData/types 等模块捕获的对象/数组引用。
export function hydrateSiteConfig(next) {
  if (!next || typeof next !== 'object') return
  replaceInPlace(siteConfig, keepProtectedAppEntries(next))
}

export default siteConfig
