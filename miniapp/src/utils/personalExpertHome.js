import { normalizeMiniappHome } from './homeMenu.js'

const DEFAULT_EXPERT = Object.freeze({
  eyebrow: '九型人格导师',
  title: '九型老师',
  lead: '用九型人格看见真实动机，把理解带进关系与成长。',
  image: '',
  monogram: '九',
})
const DEFAULT_GAME = Object.freeze({
  eyebrow: '九型探索',
  title: '人格测试',
  lead: '找到你的核心动机，开始一段更了解自己的旅程。',
  buttonText: '开始人格测试',
})
const DEFAULT_ENTERPRISE = Object.freeze({
  eyebrow: '企业共学',
  title: '让团队更懂彼此',
  lead: '以九型人格为共同语言，支持团队协作与领导力成长。',
  buttonText: '预约沟通',
  modules: Object.freeze([]),
  services: Object.freeze([{ title: '企业团队共学', description: '用九型语言帮助团队看见协作中的动机与沟通方式。' }]),
})

function isRecord(value) {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function text(value, fallback = '') {
  return typeof value === 'string' && value.trim() ? value.trim() : fallback
}

function firstText(source, keys, fallback = '') {
  for (const key of keys) {
    const value = text(source?.[key])
    if (value) return value
  }
  return fallback
}

function asItems(value) {
  if (Array.isArray(value)) return value
  if (isRecord(value) && Array.isArray(value.items)) return value.items
  if (isRecord(value) && Array.isArray(value.list)) return value.list
  return []
}

function safeMiniappHome(config, miniappHome) {
  if (isRecord(miniappHome)) return miniappHome
  return normalizeMiniappHome(config)
}

function teacherSource(config) {
  const teaser = config?.home?.teacherTeaser
  if (isRecord(teaser)) return { source: teaser, teaser: true }
  const candidates = [
    ...asItems(config?.teacher), ...asItems(config?.teachers),
    ...asItems(config?.home?.teacher), ...asItems(config?.home?.teachers),
  ]
  return { source: candidates.find(isRecord) || {}, teaser: false }
}

function normalizeExpert(config) {
  const { source, teaser } = teacherSource(config)
  if (teaser) {
    return {
      eyebrow: firstText(source, ['eyebrow'], DEFAULT_EXPERT.eyebrow),
      title: firstText(source, ['title'], DEFAULT_EXPERT.title),
      lead: firstText(source, ['lead'], DEFAULT_EXPERT.lead),
      image: firstText(source, ['image', 'fallbackImage']),
      monogram: '九',
    }
  }
  return {
    eyebrow: firstText(source, ['eyebrow', 'title', 'role', 'position', 'subtitle'], DEFAULT_EXPERT.eyebrow),
    title: firstText(source, ['name', 'teacherName', 'nickname', 'title'], DEFAULT_EXPERT.title),
    lead: firstText(source, ['lead', 'bio', 'description', 'desc', 'intro', 'summary'], DEFAULT_EXPERT.lead),
    image: firstText(source, ['image', 'avatar', 'photo', 'cover', 'fallbackImage']),
    monogram: '九',
  }
}

export function personalExpertProofStats(config) {
  try {
    const stats = config?.home?.hero?.stats
    if (!Array.isArray(stats)) return []
    return stats.reduce((result, item) => {
      if (result.length >= 3 || !isRecord(item)) return result
      const value = text(item.value)
      const label = text(item.label)
      if (!value || !label) return result
      result.push({ value, suffix: text(item.suffix), label })
      return result
    }, [])
  } catch {
    return []
  }
}

function normalizeService(item) {
  if (!isRecord(item)) return null
  const title = firstText(item, ['title', 'name', 'courseTitle'])
  const description = firstText(item, ['description', 'desc', 'lead', 'summary', 'intro'])
  if (!title && !description) return null
  return {
    title: title || '团队共学服务',
    description: description || '围绕团队协作、沟通与领导力的九型共学。',
  }
}

export function personalExpertServices(config) {
  try {
    const enterprise = isRecord(config?.home?.enterprise) ? config.home.enterprise : {}
    const configured = asItems(enterprise.items).map(normalizeService).filter(Boolean)
    const courses = asItems(config?.home?.courses?.items).map(normalizeService).filter(Boolean)
    const relevant = courses.filter((item) => /企业|团队|领导|管理|组织/.test(`${item.title} ${item.description}`))
    const services = [...configured, ...(relevant.length ? relevant : configured.length ? [] : courses)]
    return {
      eyebrow: text(enterprise.eyebrow, DEFAULT_ENTERPRISE.eyebrow),
      title: text(enterprise.title, DEFAULT_ENTERPRISE.title),
      lead: text(enterprise.lead, DEFAULT_ENTERPRISE.lead),
      buttonText: text(enterprise.buttonText, DEFAULT_ENTERPRISE.buttonText),
      modules: Array.isArray(enterprise.modules) ? enterprise.modules.map((item) => text(item)).filter(Boolean) : [],
      services: (services.length ? services : DEFAULT_ENTERPRISE.services).map((item) => ({ ...item })),
    }
  } catch {
    return { ...DEFAULT_ENTERPRISE, modules: [], services: DEFAULT_ENTERPRISE.services.map((item) => ({ ...item })) }
  }
}

export function personalExpertGameSection(config, miniappHome) {
  try {
    const miniapp = safeMiniappHome(config, miniappHome)
    const game = isRecord(config?.home?.game) ? config.home.game : {}
    const testEntry = isRecord(miniapp.testEntry) ? miniapp.testEntry : {}
    return {
      enabled: testEntry.enabled !== false,
      eyebrow: text(game.eyebrow, DEFAULT_GAME.eyebrow),
      title: text(game.title, text(testEntry.title, DEFAULT_GAME.title)),
      lead: text(game.lead, text(testEntry.description, DEFAULT_GAME.lead)),
      buttonText: text(game.buttonText, DEFAULT_GAME.buttonText),
    }
  } catch {
    return { enabled: true, ...DEFAULT_GAME }
  }
}

export function normalizePersonalExpertHome(config) {
  try {
    const miniappHome = normalizeMiniappHome(config)
    return {
      brand: { ...miniappHome.brand },
      expertHero: normalizeExpert(config),
      proofStats: personalExpertProofStats(config),
      enterprise: personalExpertServices(config),
      game: personalExpertGameSection(config, miniappHome),
      secondaryEntries: miniappHome.navigationEntries.map((item) => ({ ...item })),
      cases: [],
    }
  } catch {
    return {
      brand: { ...normalizeMiniappHome().brand },
      expertHero: { ...DEFAULT_EXPERT },
      proofStats: [],
      enterprise: personalExpertServices(),
      game: { enabled: true, ...DEFAULT_GAME },
      secondaryEntries: normalizeMiniappHome().navigationEntries.map((item) => ({ ...item })),
      cases: [],
    }
  }
}
