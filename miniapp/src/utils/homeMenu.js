export const MINIAPP_HOME_ICON_KEYS = Object.freeze([
  'compass',
  'relation',
  'book',
  'growth',
  'spark',
  'heart',
])

export const MINIAPP_HOME_THEME_KEYS = Object.freeze([
  'blue',
  'purple',
  'orange',
  'pink',
  'cyan',
])

export const MINIAPP_HOME_ENTRY_BEHAVIORS = Object.freeze({
  test: Object.freeze({
    method: 'navigateTo',
    url: '/pages/test/test',
    ariaLabel: '开始九型人格测试',
  }),
  relation: Object.freeze({
    method: 'navigateTo',
    url: '/pages/relation/relation',
    ariaLabel: '打开九型关系合盘',
  }),
  learn: Object.freeze({
    method: 'switchTab',
    url: '/pages/learn/learn',
    ariaLabel: '打开老师课程与课件',
  }),
  profile: Object.freeze({
    method: 'switchTab',
    url: '/pages/profile/profile',
    ariaLabel: '打开我的成长档案',
  }),
})

const DEFAULT_ENTRIES = Object.freeze([
  Object.freeze({ key: 'test', enabled: true, title: '人格测试', description: '找到你的核心动机', icon: 'compass', theme: 'blue' }),
  Object.freeze({ key: 'relation', enabled: true, title: '关系合盘', description: '看见彼此的互动模式', icon: 'relation', theme: 'purple' }),
  Object.freeze({ key: 'learn', enabled: true, title: '老师课程', description: '跟着课件系统学习', icon: 'book', theme: 'orange' }),
  Object.freeze({ key: 'profile', enabled: true, title: '成长档案', description: '记录你的探索轨迹', icon: 'growth', theme: 'pink' }),
])

const DEFAULTS = Object.freeze({
  brand: Object.freeze({
    enabled: true,
    name: '九型芯之力',
    tagline: '看见动机，找到成长方向',
  }),
  hero: Object.freeze({
    enabled: true,
    kicker: '老师导学 · 课程配套 · 18 题自测',
    title: '读懂自己内在的能量地图',
    description: '从核心动机出发，在老师课程中理解自己，也更从容地走进关系与成长。',
    buttonText: '开始人格测试',
  }),
  entriesSection: Object.freeze({
    enabled: true,
    title: '探索你的九型能量',
    description: '从测试、关系、课程到成长档案，选择此刻最需要的一步。',
    items: DEFAULT_ENTRIES,
  }),
  growth: Object.freeze({
    enabled: true,
    eyebrow: '老师陪伴 · 持续成长',
    title: '把测试发现带进课程练习',
    description: '跟随老师的课程与课件，让理解沉淀为真实的成长行动。',
  }),
})

const ICON_KEYS = new Set(MINIAPP_HOME_ICON_KEYS)
const THEME_KEYS = new Set(MINIAPP_HOME_THEME_KEYS)
const ENTRY_DEFAULTS = new Map(DEFAULT_ENTRIES.map((entry) => [entry.key, entry]))

function createDefaultHome() {
  return {
    brand: { ...DEFAULTS.brand },
    hero: { ...DEFAULTS.hero },
    entriesSection: {
      ...DEFAULTS.entriesSection,
      items: DEFAULT_ENTRIES.map((entry) => ({ ...entry })),
    },
    growth: { ...DEFAULTS.growth },
  }
}

function isRecord(value) {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function normalizedText(value, fallback) {
  return typeof value === 'string' && value.trim() ? value.trim() : fallback
}

function normalizedEnabled(value, fallback = true) {
  return typeof value === 'boolean' ? value : fallback
}

function normalizeEntry(value, fallback) {
  return {
    key: fallback.key,
    enabled: normalizedEnabled(value.enabled, fallback.enabled),
    title: normalizedText(value.title, fallback.title),
    description: normalizedText(value.description, fallback.description),
    icon: ICON_KEYS.has(value.icon) ? value.icon : fallback.icon,
    theme: THEME_KEYS.has(value.theme) ? value.theme : fallback.theme,
  }
}

function normalizeEntries(value) {
  const normalized = []
  const seen = new Set()

  if (Array.isArray(value)) {
    for (const candidate of value) {
      if (!isRecord(candidate) || seen.has(candidate.key)) continue
      const fallback = ENTRY_DEFAULTS.get(candidate.key)
      if (!fallback) continue
      seen.add(candidate.key)
      normalized.push(normalizeEntry(candidate, fallback))
    }
  }

  for (const fallback of DEFAULT_ENTRIES) {
    if (!seen.has(fallback.key)) normalized.push({ ...fallback })
  }
  return normalized
}

export function normalizeMiniappHome(config) {
  try {
    const source = isRecord(config?.home?.miniappHome) ? config.home.miniappHome : {}
    const brand = isRecord(source.brand) ? source.brand : {}
    const hero = isRecord(source.hero) ? source.hero : {}
    const entriesSection = isRecord(source.entriesSection) ? source.entriesSection : {}
    const growth = isRecord(source.growth) ? source.growth : {}
    const items = normalizeEntries(entriesSection.items)

    return {
      brand: {
        enabled: normalizedEnabled(brand.enabled, DEFAULTS.brand.enabled),
        name: normalizedText(brand.name, DEFAULTS.brand.name),
        tagline: normalizedText(brand.tagline, DEFAULTS.brand.tagline),
      },
      hero: {
        enabled: normalizedEnabled(hero.enabled, DEFAULTS.hero.enabled),
        kicker: normalizedText(hero.kicker, DEFAULTS.hero.kicker),
        title: normalizedText(hero.title, DEFAULTS.hero.title),
        description: normalizedText(hero.description, DEFAULTS.hero.description),
        buttonText: normalizedText(hero.buttonText, DEFAULTS.hero.buttonText),
      },
      entriesSection: {
        enabled: normalizedEnabled(entriesSection.enabled, DEFAULTS.entriesSection.enabled) && items.some((item) => item.enabled),
        title: normalizedText(entriesSection.title, DEFAULTS.entriesSection.title),
        description: normalizedText(entriesSection.description, DEFAULTS.entriesSection.description),
        items,
      },
      growth: {
        enabled: normalizedEnabled(growth.enabled, DEFAULTS.growth.enabled),
        eyebrow: normalizedText(growth.eyebrow, DEFAULTS.growth.eyebrow),
        title: normalizedText(growth.title, DEFAULTS.growth.title),
        description: normalizedText(growth.description, DEFAULTS.growth.description),
      },
    }
  } catch {
    return createDefaultHome()
  }
}
