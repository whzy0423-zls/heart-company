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
    url: '',
  },
  {
    title: '三中心与成长练习',
    description: '认识脑、心、腹三中心的反应模式，配套可执行的日常觉察练习。',
    cover: '/static/wheel.png',
    badge: '练习',
    duration: '课后练习',
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
  const candidates = [
    ...asArray(config?.teacher),
    ...asArray(config?.teachers),
    ...asArray(config?.home?.teacher),
    ...asArray(config?.home?.teachers),
  ]

  const teachers = candidates.map((item) => {
    const source = item || {}
    return {
      name: firstText(source, ['name', 'teacherName', 'nickname', 'title']) || '九型老师',
      title: firstText(source, ['title', 'role', 'position', 'subtitle']) || '九型人格导师',
      avatar: firstText(source, ['avatar', 'photo', 'image', 'cover']) || '/static/avatars/9.png',
      bio: firstText(source, ['bio', 'description', 'desc', 'intro', 'summary']) || '带你用九型人格看见真实动机，把课程内容落到每天可练习的沟通与成长里。',
      tags: normalizeTags(source.tags || source.badges || source.specialties || source.skills),
    }
  }).filter((item) => item.name || item.bio)

  return teachers.length > 0 ? teachers : DEFAULT_TEACHERS
}

function normalizeCoursewareSource(item) {
  const source = item || {}
  const title = firstText(source, ['title', 'name', 'courseTitle', 'lessonTitle'])
  const description = firstText(source, ['description', 'desc', 'summary', 'intro', 'content'])
  if (!title && !description) return null
  return {
    title: title || '课程资料',
    description: description || '老师整理的九型人格学习资料，适合课前预习和课后复盘。',
    cover: firstText(source, ['cover', 'image', 'thumb', 'poster', 'avatar']) || '/static/wheel.png',
    badge: firstText(source, ['badge', 'tag', 'type', 'label']) || '课程',
    duration: firstText(source, ['duration', 'time', 'minutes', 'length']) || '',
    url: firstText(source, ['url', 'link', 'path', 'href']) || '',
  }
}

export function normalizeCoursewareItems(config) {
  const candidates = [
    ...asArray(config?.courseware),
    ...asArray(config?.materials),
    ...asArray(config?.lessons),
    ...asArray(config?.courses),
    ...asArray(config?.home?.courseware),
    ...asArray(config?.home?.materials),
    ...asArray(config?.home?.lessons),
    ...asArray(config?.home?.courses),
  ]

  const items = uniqueByTitle(candidates.map(normalizeCoursewareSource).filter(Boolean))
  return items.length > 0 ? items : DEFAULT_COURSEWARE_ITEMS
}
