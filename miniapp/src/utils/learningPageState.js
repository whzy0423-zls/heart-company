import { normalizeCoursewareItems, normalizeTeachers } from './teacherCourseware.js'

const LEARNING_CATEGORIES = new Set(['course', 'material', 'quote'])
const LEARNING_CATEGORY_ORDER = ['course', 'material', 'quote']

function identityText(value, fallback) {
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  if (typeof value === 'string' && value.trim()) return value.trim()
  return fallback
}

function safeGet(value, key, fallback) {
  try {
    const result = value?.[key]
    return result === undefined ? fallback : result
  } catch {
    return fallback
  }
}

function canonicalPropertyKey(key) {
  if (typeof key === 'symbol') {
    let globalKey = ''
    try { globalKey = Symbol.keyFor(key) || '' } catch { globalKey = '' }
    return `symbol:${globalKey || key.description || ''}`
  }
  return `string:${String(key)}`
}

function canonicalValue(value, stack = new Map(), path = '$') {
  try {
    if (value === null) return 'null'
    if (value === undefined) return '{"$type":"undefined"}'
    if (typeof value === 'string') return JSON.stringify(value.trim().replace(/\s+/g, ' '))
    if (typeof value === 'number') {
      if (Number.isNaN(value)) return '{"$number":"NaN"}'
      if (value === Infinity) return '{"$number":"Infinity"}'
      if (value === -Infinity) return '{"$number":"-Infinity"}'
      if (Object.is(value, -0)) return '{"$number":"-0"}'
      return String(value)
    }
    if (typeof value === 'boolean') return String(value)
    if (typeof value === 'bigint') return `{"$bigint":${JSON.stringify(String(value))}}`
    if (typeof value === 'symbol') return `{"$symbol":${JSON.stringify(canonicalPropertyKey(value))}}`
    if (typeof value === 'function') return `{"$function":${JSON.stringify(identityText(value.name, 'anonymous'))}}`
    if (stack.has(value)) return `{"$cycle":${JSON.stringify(stack.get(value))}}`

    stack.set(value, path)
    try {
      let tag = ''
      try { tag = Object.prototype.toString.call(value) } catch { tag = '[object Unknown]' }
      if (tag === '[object Date]') {
        let timestamp = NaN
        try { timestamp = value.getTime() } catch { timestamp = NaN }
        return `{"$date":${JSON.stringify(Number.isFinite(timestamp) ? new Date(timestamp).toISOString() : 'invalid')}}`
      }
      if (tag === '[object Map]') {
        let entries
        try { entries = Array.from(value.entries()) } catch { return '{"$error":"map-read"}' }
        return `{"$map":[${entries.map(([key, item], index) => (
          `${canonicalValue(key, stack, `${path}.map-key-${index}`)}:${canonicalValue(item, stack, `${path}.map-value-${index}`)}`
        )).sort().join(',')}]}`
      }
      if (tag === '[object Set]') {
        let entries
        try { entries = Array.from(value.values()) } catch { return '{"$error":"set-read"}' }
        return `{"$set":[${entries.map((item, index) => canonicalValue(item, stack, `${path}.set-${index}`)).sort().join(',')}]}`
      }
      if (Array.isArray(value)) {
        const length = Number(safeGet(value, 'length', 0)) || 0
        const items = []
        for (let index = 0; index < length; index += 1) {
          items.push(canonicalValue(safeGet(value, index, { $error: 'array-read' }), stack, `${path}[${index}]`))
        }
        return `[${items.sort().join(',')}]`
      }

      let keys
      try { keys = Reflect.ownKeys(value) } catch { return '{"$error":"object-keys"}' }
      const properties = keys.map((key) => {
        const canonicalKey = canonicalPropertyKey(key)
        let property
        try { property = value[key] } catch { property = { $error: 'property-read' } }
        return `${JSON.stringify(canonicalKey)}:${canonicalValue(property, stack, `${path}.${canonicalKey}`)}`
      }).sort()
      return `{${properties.join(',')}}`
    } finally {
      stack.delete(value)
    }
  } catch {
    return '{"$error":"canonicalize"}'
  }
}

function stableKey(namespace, explicitIdentity, content) {
  return `${namespace}::${canonicalValue(explicitIdentity)}::${canonicalValue(content)}`
}

function courseIdentity(course) {
  const explicit = identityText(
    safeGet(course, 'id', '') || safeGet(course, 'key', '') || safeGet(course, 'courseId', ''),
    '',
  )
  return stableKey('course', explicit || null, course)
}

function materialIdentity(material) {
  if (typeof material === 'string') {
    const text = material.trim()
    return text ? { id: text, label: text } : null
  }
  if (!material || typeof material !== 'object') return null
  const label = identityText(
    safeGet(material, 'name', '') || safeGet(material, 'title', '') || safeGet(material, 'type', '') || safeGet(material, 'label', ''),
    '',
  )
  if (!label) return null
  const explicit = identityText(
    safeGet(material, 'id', '') || safeGet(material, 'key', '') || safeGet(material, 'code', ''),
    '',
  )
  return {
    id: stableKey('material', explicit || null, material),
    label,
  }
}

function withUniqueKeys(entries) {
  const counts = new Map()
  return entries.map((entry) => {
    const occurrence = (counts.get(entry.baseKey) || 0) + 1
    counts.set(entry.baseKey, occurrence)
    return {
      ...entry,
      key: occurrence === 1 ? entry.baseKey : `${entry.baseKey}::duplicate::${occurrence}`,
    }
  })
}

export function createLearningCourseEntries(courses) {
  if (!Array.isArray(courses)) return []
  return withUniqueKeys(courses.flatMap((course) => {
    if (!course || typeof course !== 'object') return []
    return [{ baseKey: courseIdentity(course), item: course }]
  })).map(({ baseKey, ...entry }) => entry)
}

export function createLearningQuoteEntries(quotes) {
  if (!Array.isArray(quotes)) return []
  return withUniqueKeys(quotes.flatMap((quote) => {
    const text = identityText(quote, '')
    return text ? [{ baseKey: stableKey('quote', null, text), text }] : []
  })).map(({ baseKey, ...entry }) => entry)
}

export function createLearningTagEntries(tags, ownerIdentity = '') {
  if (!Array.isArray(tags)) return []
  return withUniqueKeys(tags.flatMap((tag) => {
    const text = identityText(tag, '')
    return text ? [{ baseKey: stableKey('tag', ownerIdentity || null, text), text }] : []
  })).map(({ baseKey, ...entry }) => entry)
}

export function flattenLearningMaterials(courses) {
  const materials = createLearningCourseEntries(courses).flatMap(({ key: courseKey, item: course }) => {
    const materialTypes = safeGet(course, 'materialTypes', null)
    if (!Array.isArray(materialTypes)) return []
    const courseTitle = identityText(safeGet(course, 'title', '') || safeGet(course, 'name', ''), '未命名课程')
    return materialTypes.map((material) => {
      const identity = materialIdentity(material)
      if (!identity) return null
      return {
        baseKey: `${courseKey}::material::${identity.id}`,
        courseKey,
        courseTitle,
        type: identity.label,
        description: identityText(safeGet(course, 'description', ''), ''),
        duration: identityText(safeGet(course, 'duration', ''), ''),
        url: identityText(safeGet(material, 'url', '') || safeGet(course, 'url', ''), ''),
      }
    }).filter(Boolean)
  })
  return withUniqueKeys(materials).map(({ baseKey, ...entry }) => entry)
}

export function resolveLearningCategory(currentCategory, navigationIntent) {
  if (LEARNING_CATEGORIES.has(navigationIntent)) return navigationIntent
  return LEARNING_CATEGORIES.has(currentCategory) ? currentCategory : 'course'
}

export function learningTabTransition(currentCategory, key) {
  const category = resolveLearningCategory(currentCategory, null)
  const currentIndex = LEARNING_CATEGORY_ORDER.indexOf(category)
  if (['Enter', ' ', 'Spacebar'].includes(key)) return { handled: true, category, focusIndex: currentIndex }
  if (!['ArrowLeft', 'ArrowRight'].includes(key)) return { handled: false, category, focusIndex: currentIndex }
  const step = key === 'ArrowRight' ? 1 : -1
  const focusIndex = (currentIndex + step + LEARNING_CATEGORY_ORDER.length) % LEARNING_CATEGORY_ORDER.length
  return { handled: true, category: LEARNING_CATEGORY_ORDER[focusIndex], focusIndex }
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
