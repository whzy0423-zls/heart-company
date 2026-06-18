import bundledConfig from '../../../shared/site-config.json'

// 站点配置：构建时内置一份默认值作兜底，运行时由 main.jsx 在渲染前
// 用后台公开接口拉取的最新配置原地填充（hydrate）。
// 所有消费方（navData.js / types.js / 各组件）只要在 hydrate 之后被加载，
// 读到的就是最新配置，无需改动它们。
const siteConfig = structuredClone(bundledConfig)

// 用后台返回的数据原地覆盖（保持对象引用不变，便于已捕获引用的模块生效）。
export function hydrateSiteConfig(next) {
  if (!next || typeof next !== 'object') return
  // 逐键覆盖顶层字段（site / navigation / home / types ...）
  for (const key of Object.keys(next)) {
    siteConfig[key] = next[key]
  }
}

export default siteConfig
