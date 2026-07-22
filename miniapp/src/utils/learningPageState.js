import { normalizeCoursewareItems, normalizeTeachers } from './teacherCourseware.js'

const LEARNING_CATEGORIES = new Set(['course', 'material', 'quote'])

function identityText(value, fallback) {
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  if (typeof value === 'string' && value.trim()) return value.trim()
  return fallback
}

function materialIdentity(material, index) {
  if (typeof material === 'string') {
    const text = material.trim()
    return text ? { id: text, label: text } : null
  }
  if (!material || typeof material !== 'object') return null
  const label = identityText(material.name || material.title || material.type || material.label, '')
  if (!label) return null
  return {
    id: identityText(material.id || material.key || material.code, label || `material-${index}`),
    label,
  }
}

export function flattenLearningMaterials(courses) {
  if (!Array.isArray(courses)) return []
  const keyCounts = new Map()
  return courses.flatMap((course, courseIndex) => {
    if (!course || typeof course !== 'object' || !Array.isArray(course.materialTypes)) return []
    const rawCourseTitle = identityText(course.title || course.name, '')
    const courseTitle = rawCourseTitle || '未命名课程'
    const courseId = identityText(course.id || course.key || course.courseId, rawCourseTitle || `course-${courseIndex}`)
    return course.materialTypes.map((material, materialIndex) => {
      const identity = materialIdentity(material, materialIndex)
      if (!identity) return null
      const baseKey = `${courseId}::material::${identity.id}`
      const occurrence = (keyCounts.get(baseKey) || 0) + 1
      keyCounts.set(baseKey, occurrence)
      return {
        key: occurrence === 1 ? baseKey : `${baseKey}::${occurrence}`,
        courseKey: courseId,
        courseTitle,
        type: identity.label,
        description: identityText(course.description, ''),
        duration: identityText(course.duration, ''),
        url: identityText(material?.url || course.url, ''),
      }
    }).filter(Boolean)
  })
}

export function resolveLearningCategory(currentCategory, navigationIntent) {
  if (LEARNING_CATEGORIES.has(navigationIntent)) return navigationIntent
  return LEARNING_CATEGORIES.has(currentCategory) ? currentCategory : 'course'
}

export const TEACHER_SECTION_PATHS = [
  ['teacher'],
  ['teachers'],
  ['home', 'teacher'],
  ['home', 'teachers'],
  ['home', 'teacherTeaser'],
]

export const COURSE_SECTION_PATHS = [
  ['courseware'],
  ['materials'],
  ['lessons'],
  ['courses'],
  ['home', 'courseware'],
  ['home', 'materials'],
  ['home', 'lessons'],
  ['home', 'courses'],
]

export const QUOTE_SECTION_PATHS = [
  ['quotes'],
  ['home', 'quotes'],
]

export function hasSectionAtPath(config, path) {
  let current = config
  for (let index = 0; index < path.length; index += 1) {
    if (!current || typeof current !== 'object') return false
    const key = path[index]
    if (!Object.prototype.hasOwnProperty.call(current, key)) return false
    if (index === path.length - 1) return true
    current = current[key]
  }
  return false
}

function hasSection(config, paths) {
  return paths.some((path) => hasSectionAtPath(config, path))
}

function quoteItems(value) {
  const items = Array.isArray(value)
    ? value
    : Array.isArray(value?.items)
      ? value.items
      : Array.isArray(value?.list)
        ? value.list
        : []
  return items.map((item) => {
    if (typeof item === 'string') return item.trim()
    const text = item?.quote || item?.text || item?.content || ''
    return typeof text === 'string' ? text.trim() : ''
  }).filter(Boolean)
}

export function normalizeLearningQuotes(config) {
  if (hasSectionAtPath(config, ['quotes'])) return quoteItems(config?.quotes)
  return quoteItems(config?.home?.quotes)
}

export function createInitialLearningContent() {
  return {
    teachers: normalizeTeachers(),
    coursewareItems: normalizeCoursewareItems(),
    quotes: [],
  }
}

export function applyLearningContent(current, config, options = {}) {
  const preserveMissing = !!options.preserveMissing
  return {
    ...current,
    teachers: !preserveMissing || hasSection(config, TEACHER_SECTION_PATHS)
      ? normalizeTeachers(config)
      : current.teachers,
    coursewareItems: !preserveMissing || hasSection(config, COURSE_SECTION_PATHS)
      ? normalizeCoursewareItems(config)
      : current.coursewareItems,
    quotes: !preserveMissing || hasSection(config, QUOTE_SECTION_PATHS)
      ? normalizeLearningQuotes(config)
      : current.quotes,
  }
}

export function retainLearningContentOnError(current, loadError) {
  return { ...current, loadError }
}

export function createLatestRequestGuard() {
  let latestTicket = 0
  return {
    issue() {
      latestTicket += 1
      return latestTicket
    },
    isLatest(ticket) {
      return ticket === latestTicket
    },
  }
}

export function createOneShotFallbackRegistry() {
  const used = new Set()
  return {
    consume(key) {
      if (used.has(key)) return false
      used.add(key)
      return true
    },
    reset() {
      used.clear()
    },
  }
}

export function createActionActivationGuard(options = {}) {
  const now = options.now || (() => Date.now())
  const suppressionMs = options.suppressionMs || 500
  let keyboardActivationAt = 0
  let keyboardActivationTarget = null

  return {
    shouldActivate(event) {
      const eventType = event?.type || ''
      const timestamp = now()
      if (eventType === 'keydown') {
        if (event?.repeat) return false
        keyboardActivationAt = timestamp
        keyboardActivationTarget = event?.currentTarget || null
        return true
      }
      if (
        eventType === 'click'
        && keyboardActivationTarget === (event?.currentTarget || null)
        && timestamp - keyboardActivationAt < suppressionMs
      ) {
        keyboardActivationTarget = null
        return false
      }
      keyboardActivationTarget = null
      return true
    },
  }
}

export function handleActionKeydown(event, activate) {
  if (!['Enter', ' ', 'Spacebar'].includes(event?.key)) return false
  event.preventDefault?.()
  event.stopPropagation?.()
  activate(event)
  return true
}
