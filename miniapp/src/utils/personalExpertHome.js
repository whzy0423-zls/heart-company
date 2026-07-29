import { normalizeMiniappHome } from './homeMenu.js'
import { resolveContentAsset } from './contentAsset.js'

const DEFAULT_EXPERT_PORTRAIT_IMAGE = '/assets/teacher.jpg'
const DEFAULT_EXPERT_DETAIL_IMAGE = '/assets/teacher-poster.jpg'
const DEFAULT_EXPERT = Object.freeze({
  eyebrow: '九型人格导师',
  title: '九型老师',
  lead: '用九型人格看见真实动机，把理解带进关系与成长。',
  monogram: '九',
})
const DEFAULT_GAME = Object.freeze({
  eyebrow: '九型探索',
  title: '人格测试',
  lead: '找到你的核心动机，开始一段更了解自己的旅程。',
  buttonText: '开始人格测试',
})
const DEFAULT_ENTERPRISE_SERVICES = Object.freeze([
  { title: '企业团队共学', description: '用九型语言帮助团队看见协作中的动机与沟通方式。' },
])
const DEFAULT_ENTERPRISE_SERVICE_MODES = Object.freeze([
  { title: '企业内训', description: '围绕企业当下议题设计半天或全天共学。' },
  { title: '团队工作坊', description: '用互动练习帮助团队建立沟通和协作共识。' },
  { title: '管理者培训', description: '支持管理者识别不同类型成员的动机与压力反应。' },
])
const DEFAULT_ENTERPRISE_PROCESS_STEPS = Object.freeze([
  { title: '需求沟通', description: '先了解团队背景、参与对象和希望解决的问题。' },
  { title: '方案共创', description: '结合九型主题、课件内容和企业节奏设计服务方式。' },
  { title: '落地交付', description: '完成课程或工作坊后，沉淀可复盘的团队语言。' },
])
const DEFAULT_ENTERPRISE = Object.freeze({
  eyebrow: '企业共学',
  title: '让团队更懂彼此',
  lead: '以九型人格为共同语言，支持团队协作与领导力成长。',
  buttonText: '预约沟通',
  modules: Object.freeze([]),
  services: DEFAULT_ENTERPRISE_SERVICES,
  serviceModes: DEFAULT_ENTERPRISE_SERVICE_MODES,
  processSteps: DEFAULT_ENTERPRISE_PROCESS_STEPS,
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

function firstAsset(source, keys, fallback = '') {
  for (const key of keys) {
    const resolved = resolveContentAsset(text(source?.[key]))
    if (resolved) return resolved
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
  const portraitKeys = teaser
    ? ['portraitImage', 'avatar', 'photo']
    : ['portraitImage', 'avatar', 'photo', 'cover']
  const portraitImage = firstAsset(source, portraitKeys, resolveContentAsset(DEFAULT_EXPERT_PORTRAIT_IMAGE))
  const detailImage = firstAsset(source, ['detailImage', 'poster', 'image', 'cover', 'fallbackImage'], resolveContentAsset(DEFAULT_EXPERT_DETAIL_IMAGE))
  if (teaser) {
    return {
      eyebrow: firstText(source, ['eyebrow'], DEFAULT_EXPERT.eyebrow),
      title: firstText(source, ['title'], DEFAULT_EXPERT.title),
      lead: firstText(source, ['lead'], DEFAULT_EXPERT.lead),
      portraitImage,
      detailImage,
      image: detailImage,
      monogram: '九',
    }
  }
  return {
    eyebrow: firstText(source, ['eyebrow', 'title', 'role', 'position', 'subtitle'], DEFAULT_EXPERT.eyebrow),
    title: firstText(source, ['name', 'teacherName', 'nickname', 'title'], DEFAULT_EXPERT.title),
    lead: firstText(source, ['lead', 'bio', 'description', 'desc', 'intro', 'summary'], DEFAULT_EXPERT.lead),
    portraitImage,
    detailImage,
    image: detailImage,
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

function normalizeEnterpriseBookingItems(value, defaults) {
  const items = asItems(value).map(normalizeService).filter(Boolean).slice(0, 4)
  return (items.length ? items : defaults).map((item) => ({ ...item }))
}

function freshEnterpriseDefaults() {
  return {
    ...DEFAULT_ENTERPRISE,
    modules: [],
    services: DEFAULT_ENTERPRISE.services.map((item) => ({ ...item })),
    serviceModes: DEFAULT_ENTERPRISE.serviceModes.map((item) => ({ ...item })),
    processSteps: DEFAULT_ENTERPRISE.processSteps.map((item) => ({ ...item })),
  }
}

function enterpriseCourseText(source) {
  const fields = ['title', 'description', 'summary', 'tag', 'tags', 'type', 'category', 'label', 'badge']
  return fields.flatMap((field) => {
    const value = source?.[field]
    return Array.isArray(value) ? value : [value]
  }).filter((value) => typeof value === 'string').join(' ').toLowerCase()
}

function isEnterpriseCourse(source) {
  return /企业|团队|组织|领导|工作坊|enterprise|corporate|team|leadership|organization|workshop/i.test(enterpriseCourseText(source))
}

export function personalExpertServices(config) {
  try {
    const enterprise = isRecord(config?.home?.enterprise) ? config.home.enterprise : {}
    const configured = asItems(enterprise.items).map(normalizeService).filter(Boolean).slice(0, 3)
    const relevantCourses = asItems(config?.home?.courses?.items)
      .filter(isEnterpriseCourse)
      .map(normalizeService)
      .filter(Boolean)
      .slice(0, 3)
    const services = [...configured, ...relevantCourses].slice(0, 3)
    return {
      eyebrow: text(enterprise.eyebrow, DEFAULT_ENTERPRISE.eyebrow),
      title: text(enterprise.title, DEFAULT_ENTERPRISE.title),
      lead: text(enterprise.lead, DEFAULT_ENTERPRISE.lead),
      buttonText: text(enterprise.buttonText, DEFAULT_ENTERPRISE.buttonText),
      modules: Array.isArray(enterprise.modules) ? enterprise.modules.map((item) => text(item)).filter(Boolean).slice(0, 4) : [],
      services: (services.length ? services : DEFAULT_ENTERPRISE.services).map((item) => ({ ...item })),
      serviceModes: normalizeEnterpriseBookingItems(enterprise.items, DEFAULT_ENTERPRISE.serviceModes),
      processSteps: normalizeEnterpriseBookingItems(enterprise.processSteps, DEFAULT_ENTERPRISE.processSteps),
    }
  } catch {
    return freshEnterpriseDefaults()
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
    const portraitImage = resolveContentAsset(DEFAULT_EXPERT_PORTRAIT_IMAGE)
    const detailImage = resolveContentAsset(DEFAULT_EXPERT_DETAIL_IMAGE)
    return {
      brand: { ...normalizeMiniappHome().brand },
      expertHero: { ...DEFAULT_EXPERT, portraitImage, detailImage, image: detailImage },
      proofStats: [],
      enterprise: personalExpertServices(),
      game: { enabled: true, ...DEFAULT_GAME },
      secondaryEntries: normalizeMiniappHome().navigationEntries.map((item) => ({ ...item })),
      cases: [],
    }
  }
}
