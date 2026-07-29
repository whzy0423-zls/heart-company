export const DEFAULT_MINIAPP_LEARN = Object.freeze({
  hero: Object.freeze({
    eyebrow: '老师课堂',
    title: '跟着老师，把九型真正用进工作与生活',
    lead: '从视频与音频课件开始，理解自己、改善关系，也为团队协作建立更清晰的共同语言。',
    meta: Object.freeze(['视频课程', '音频精讲', '九型实践']),
  }),
  classroom: Object.freeze({
    eyebrow: '课堂精选',
    title: '视频与音频课件',
    moreText: '查看全部',
    heroEyebrow: '随时回看 · 反复练习',
    heroTitle: '把老师以往开课内容，整理成可以持续学习的专业课件',
    heroLead: '支持视频和音频；先看独立课件，也可以进入系列课程循序学习。',
    ctaText: '进入老师课堂',
    emptyTitle: '老师课堂正在准备中',
    emptyDescription: '可以先浏览老师介绍和课程方向，新的视频与音频课件会在这里持续更新。',
    emptyActionText: '进入课堂看看',
  }),
  sections: Object.freeze({
    teacher: Object.freeze({ eyebrow: '老师简介', title: '认识你的学习向导' }),
    courses: Object.freeze({
      eyebrow: '课程方向',
      title: '循序建立九型视角',
      emptyTitle: '课程方向正在整理中',
      emptyDescription: '更多面向个人成长、关系沟通与企业团队的学习主题会持续补充。',
    }),
    types: Object.freeze({ eyebrow: '九型内容', title: '九种性格，九条成长路径' }),
    quotes: Object.freeze({
      eyebrow: '课堂一念',
      title: '把觉察带回当下',
      emptyTitle: '课堂语录即将上线',
    }),
  }),
  bottomCtaText: '先完成测试，建立你的学习地图',
})

function isRecord(value) {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}

function text(value, fallback) {
  return typeof value === 'string' && value.trim() ? value.trim() : fallback
}

function normalizeMeta(value) {
  if (!Array.isArray(value)) return [...DEFAULT_MINIAPP_LEARN.hero.meta]
  const items = value
    .filter((item) => typeof item === 'string' && item.trim())
    .map((item) => item.trim())
    .slice(0, 3)
  return items.length ? items : [...DEFAULT_MINIAPP_LEARN.hero.meta]
}

function normalizeSection(value, fallback, optionalTextFields = []) {
  const source = isRecord(value) ? value : {}
  return optionalTextFields.reduce(
    (section, field) => ({ ...section, [field]: text(source[field], fallback[field]) }),
    {
      eyebrow: text(source.eyebrow, fallback.eyebrow),
      title: text(source.title, fallback.title),
    },
  )
}

function createDefaults() {
  return {
    hero: { ...DEFAULT_MINIAPP_LEARN.hero, meta: [...DEFAULT_MINIAPP_LEARN.hero.meta] },
    classroom: { ...DEFAULT_MINIAPP_LEARN.classroom },
    sections: {
      teacher: { ...DEFAULT_MINIAPP_LEARN.sections.teacher },
      courses: { ...DEFAULT_MINIAPP_LEARN.sections.courses },
      types: { ...DEFAULT_MINIAPP_LEARN.sections.types },
      quotes: { ...DEFAULT_MINIAPP_LEARN.sections.quotes },
    },
    bottomCtaText: DEFAULT_MINIAPP_LEARN.bottomCtaText,
  }
}

export function normalizeMiniappLearn(config) {
  try {
    const source = isRecord(config?.home?.miniappLearn) ? config.home.miniappLearn : {}
    const hero = isRecord(source.hero) ? source.hero : {}
    const classroom = isRecord(source.classroom) ? source.classroom : {}
    const sections = isRecord(source.sections) ? source.sections : {}

    return {
      hero: {
        eyebrow: text(hero.eyebrow, DEFAULT_MINIAPP_LEARN.hero.eyebrow),
        title: text(hero.title, DEFAULT_MINIAPP_LEARN.hero.title),
        lead: text(hero.lead, DEFAULT_MINIAPP_LEARN.hero.lead),
        meta: normalizeMeta(hero.meta),
      },
      classroom: {
        eyebrow: text(classroom.eyebrow, DEFAULT_MINIAPP_LEARN.classroom.eyebrow),
        title: text(classroom.title, DEFAULT_MINIAPP_LEARN.classroom.title),
        moreText: text(classroom.moreText, DEFAULT_MINIAPP_LEARN.classroom.moreText),
        heroEyebrow: text(classroom.heroEyebrow, DEFAULT_MINIAPP_LEARN.classroom.heroEyebrow),
        heroTitle: text(classroom.heroTitle, DEFAULT_MINIAPP_LEARN.classroom.heroTitle),
        heroLead: text(classroom.heroLead, DEFAULT_MINIAPP_LEARN.classroom.heroLead),
        ctaText: text(classroom.ctaText, DEFAULT_MINIAPP_LEARN.classroom.ctaText),
        emptyTitle: text(classroom.emptyTitle, DEFAULT_MINIAPP_LEARN.classroom.emptyTitle),
        emptyDescription: text(classroom.emptyDescription, DEFAULT_MINIAPP_LEARN.classroom.emptyDescription),
        emptyActionText: text(classroom.emptyActionText, DEFAULT_MINIAPP_LEARN.classroom.emptyActionText),
      },
      sections: {
        teacher: normalizeSection(sections.teacher, DEFAULT_MINIAPP_LEARN.sections.teacher),
        courses: normalizeSection(sections.courses, DEFAULT_MINIAPP_LEARN.sections.courses, [
          'emptyTitle',
          'emptyDescription',
        ]),
        types: normalizeSection(sections.types, DEFAULT_MINIAPP_LEARN.sections.types),
        quotes: normalizeSection(sections.quotes, DEFAULT_MINIAPP_LEARN.sections.quotes, ['emptyTitle']),
      },
      bottomCtaText: text(source.bottomCtaText, DEFAULT_MINIAPP_LEARN.bottomCtaText),
    }
  } catch {
    return createDefaults()
  }
}
