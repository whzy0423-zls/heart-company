import siteConfig from './siteConfig'

// 保持旧数组结构，避免 Wheel 等组件需要同步大改。
export const TYPES = siteConfig.types.map((item) => [
  item.id,
  item.name,
  item.keywords,
  item.description,
])
