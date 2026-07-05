import bundledConfig from '../../../shared/site-config.json'

// 站点配置：构建时内置一份默认值作兜底，运行时由 main.jsx 在渲染前
// 用后台公开接口拉取的最新配置原地填充（hydrate）。
// 所有消费方（navData.js / types.js / 各组件）只要在 hydrate 之后被加载，
// 读到的就是最新配置，无需改动它们。
const siteConfig = structuredClone(bundledConfig)

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
  replaceInPlace(siteConfig, next)
}

export default siteConfig
