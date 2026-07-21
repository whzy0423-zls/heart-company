export const DEFAULT_TEACHERS = [
  {
    name: '韩老师',
    title: '九型人格主讲老师',
    avatar: '/static/avatars/9.png',
    bio: '长期带领九型人格学习与个案梳理，擅长把类型动机、关系沟通和日常成长练习讲清楚。',
    tags: ['九型入门', '关系沟通', '成长练习'],
  },
]

export const DEFAULT_COURSEWARE_ITEMS = [
  {
    title: '九型人格入门课件',
    description: '从九种核心动机、恐惧与注意力习惯开始，建立学习九型的基础地图。',
    cover: '/static/wheel.png',
    badge: '入门',
    duration: '约 20 分钟',
    materialTypes: ['课件'],
    bullets: [],
    url: '',
  },
  {
    title: '三中心与成长练习',
    description: '认识脑、心、腹三中心的反应模式，配套可执行的日常觉察练习。',
    cover: '/static/wheel.png',
    badge: '练习',
    duration: '课后练习',
    materialTypes: ['课件', '练习'],
    bullets: [],
    url: '',
  },
]

function asArray(value) {
  if (!value) return []
  if (Array.isArray(value)) return value
  if (Array.isArray(value.items)) return value.items
  if (Array.isArray(value.list)) return value.list
  return typeof value === 'object' ? [value] : []
}

function hasExplicitSection(value) {
  return Array.isArray(value)
    || Array.isArray(value?.items)
    || Array.isArray(value?.list)
    || !!(value && typeof value === 'object')
}

function firstText(source, keys) {
  for (const key of keys) {
    const value = source?.[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
    if (typeof value === 'number') return String(value)
  }
  return ''
}

function normalizeTags(value) {
  if (Array.isArray(value)) return value.map((item) => String(item).trim()).filter(Boolean).slice(0, 4)
  if (typeof value === 'string') return value.split(/[、,，/\s]+/).map((item) => item.trim()).filter(Boolean).slice(0, 4)
  return []
}

function normalizeTextList(value) {
  if (Array.isArray(value)) return value.map((item) => String(item).trim()).filter(Boolean)
  if (typeof value === 'string') return value.split(/[、,，/]+/).map((item) => item.trim()).filter(Boolean)
  return []
}

function meaningfulEyebrow(value) {
  const eyebrow = typeof value === 'string' ? value.trim() : ''
  if (!eyebrow || /^(老师|导师|师资)(简介|介绍)?$/.test(eyebrow)) return ''
  return eyebrow
}

function teacherNameAndTitle(source) {
  const explicitName = firstText(source, ['name', 'teacherName', 'nickname'])
  if (explicitName) {
    return {
      name: explicitName,
      title: firstText(source, ['title', 'role', 'position', 'subtitle']) || meaningfulEyebrow(source?.eyebrow) || '九型人格导师',
    }
  }

  const combinedTitle = firstText(source, ['title'])
  const [name, ...identityParts] = combinedTitle.split(/[｜|]/).map((part) => part.trim()).filter(Boolean)
  return {
    name: name || '九型老师',
    title: identityParts.join('｜') || meaningfulEyebrow(source?.eyebrow) || '九型人格导师',
  }
}

function hasTeacherContent(source) {
  if (!source || typeof source !== 'object') return false
  return !!firstText(source, [
    'name', 'teacherName', 'nickname', 'title', 'role', 'position', 'subtitle',
    'avatar', 'photo', 'image', 'cover', 'bio', 'description', 'desc', 'intro', 'summary', 'lead',
  ]) || normalizeTags(source.tags || source.badges || source.specialties || source.skills).length > 0
}

function uniqueByTitle(items) {
  const seen = new Set()
  return items.filter((item) => {
    const key = `${item.title || item.name}|${item.description || item.bio || ''}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

export function normalizeTeachers(config) {
  const sources = [
    config?.teacher,
    config?.teachers,
    config?.home?.teacher,
    config?.home?.teachers,
    config?.home?.teacherTeaser,
  ]
  const candidates = sources.flatMap(asArray).filter(hasTeacherContent)

  const teachers = candidates.map((item) => {
    const source = item || {}
    const identity = teacherNameAndTitle(source)
    return {
      name: identity.name,
      title: identity.title,
      avatar: firstText(source, ['avatar', 'photo', 'image', 'cover']) || '/static/avatars/9.png',
      bio: firstText(source, ['bio', 'description', 'desc', 'intro', 'summary', 'lead']) || '带你用九型人格看见真实动机，把课程内容落到每天可练习的沟通与成长里。',
      tags: normalizeTags(source.tags || source.badges || source.specialties || source.skills),
    }
  }).filter((item) => item.name || item.bio)

  if (teachers.length > 0) return teachers
  return sources.some(hasExplicitSection) ? [] : DEFAULT_TEACHERS
}

const COURSE_EDITORIAL = [
  {
    match: /个人|成长|疗愈/,
    cover: '/static/editorial/course-growth.webp',
    materialTypes: ['课件', '音频'],
    duration: '6 讲 · 约 90 分钟',
  },
  {
    match: /领导|团队|企业|组织/,
    cover: '/static/editorial/course-intro.webp',
    materialTypes: ['课件', '视频'],
    duration: '8 讲 · 约 120 分钟',
  },
  {
    match: /家庭|亲子|夫妻|婚姻|系统排列/,
    cover: '/static/editorial/course-relation.webp',
    materialTypes: ['课件', '视频', '音频'],
    duration: '5 讲 · 约 75 分钟',
  },
]

function courseEditorial(title, index) {
  return COURSE_EDITORIAL.find((item) => item.match.test(title)) || COURSE_EDITORIAL[index % COURSE_EDITORIAL.length]
}

function normalizeCoursewareSource(item, index) {
  const source = item || {}
  const title = firstText(source, ['title', 'name', 'courseTitle', 'lessonTitle'])
  const description = firstText(source, ['description', 'desc', 'summary', 'intro', 'content'])
  if (!title && !description) return null
  const editorial = courseEditorial(title, index)
  const materialTypes = normalizeTextList(source.materialTypes || source.mediaTypes || source.formats)
  return {
    title: title || '课程资料',
    description: description || '老师整理的九型人格学习资料，适合课前预习和课后复盘。',
    cover: firstText(source, ['cover', 'image', 'thumb', 'poster', 'avatar']) || editorial.cover,
    badge: firstText(source, ['badge', 'tag', 'type', 'label']) || '课程',
    duration: firstText(source, ['duration', 'time', 'minutes', 'length']) || editorial.duration,
    materialTypes: materialTypes.length > 0 ? materialTypes : editorial.materialTypes,
    bullets: normalizeTextList(source.bullets),
    url: firstText(source, ['url', 'link', 'path', 'href']) || '',
  }
}

export function normalizeCoursewareItems(config) {
  const sources = [
    config?.courseware,
    config?.materials,
    config?.lessons,
    config?.courses,
    config?.home?.courseware,
    config?.home?.materials,
    config?.home?.lessons,
    config?.home?.courses,
  ]
  const candidates = sources.flatMap(asArray)

  const items = uniqueByTitle(candidates.map(normalizeCoursewareSource).filter(Boolean))
  if (items.length > 0) return items
  return sources.some(hasExplicitSection) ? [] : DEFAULT_COURSEWARE_ITEMS
}
