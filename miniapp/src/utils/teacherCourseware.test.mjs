import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-teacher-courseware-'))
const modulePath = join(dir, 'teacherCourseware.mjs')
let source = await readFile(new URL('./teacherCourseware.js', import.meta.url), 'utf8')
await writeFile(modulePath, source)

const {
  DEFAULT_COURSEWARE_ITEMS,
  DEFAULT_TEACHERS,
  normalizeCoursewareItems,
  normalizeTeachers,
} = await import(`file://${modulePath}`)

assert.ok(DEFAULT_TEACHERS.length > 0, 'stable fallback teachers should be available')
assert.ok(DEFAULT_COURSEWARE_ITEMS.length > 0, 'stable fallback courseware should be available')
const courseSchemaKeys = ['badge', 'bullets', 'cover', 'description', 'duration', 'materialTypes', 'title', 'url']
for (const item of DEFAULT_COURSEWARE_ITEMS) {
  assert.deepEqual(Object.keys(item).sort(), courseSchemaKeys, 'default courseware should expose the complete normalized schema')
  assert.equal(Array.isArray(item.bullets), true, 'default courseware bullets should always be an array')
  assert.equal(Array.isArray(item.materialTypes), true, 'default courseware materialTypes should always be an array')
}

assert.deepEqual(
  normalizeTeachers({ teacher: { name: '韩老师', title: '九型导师', avatar: '/a.png', bio: '十年咨询经验', tags: ['课程研发'] } })[0],
  { name: '韩老师', title: '九型导师', avatar: '/a.png', bio: '十年咨询经验', tags: ['课程研发'] },
  'root teacher object should be normalized',
)
assert.equal(
  normalizeTeachers({ home: { teachers: [{ name: 'A' }, { nickname: 'B', role: '讲师' }] } }).length,
  2,
  'home.teachers array should be normalized',
)
assert.equal(
  normalizeTeachers({ teachers: { items: [{ teacherName: '李老师', desc: '授课老师' }] } })[0].name,
  '李老师',
  'teachers.items should accept teacherName aliases',
)
assert.deepEqual(normalizeTeachers({ home: {} }), DEFAULT_TEACHERS, 'missing teachers should use stable defaults')
assert.deepEqual(normalizeTeachers({ home: { teacherTeaser: {} } }), [], 'an explicit empty teacherTeaser should remain empty')
assert.equal(
  normalizeTeachers({ teacher: { name: '优先老师' }, home: { teacherTeaser: {} } })[0].name,
  '优先老师',
  'a nonempty higher-priority teacher source should win over an empty teacherTeaser',
)

const teacherTeaser = {
  eyebrow: '老师简介',
  title: '韩常青（老韩）｜九型芯之力首席导师',
  lead: '北京九型成长平台、芯之力创始人。',
  image: '/assets/teacher-poster.jpg',
}
assert.deepEqual(
  normalizeTeachers({ home: { teacherTeaser } })[0],
  {
    name: '韩常青（老韩）',
    title: '九型芯之力首席导师',
    avatar: '/assets/teacher-poster.jpg',
    bio: '北京九型成长平台、芯之力创始人。',
    tags: [],
  },
  'real teacherTeaser content should split the combined title without duplicating it',
)
assert.deepEqual(
  normalizeTeachers({ home: { teacherTeaser: { title: '韩常青（老韩） | 首席导师', eyebrow: '老师简介' } } })[0],
  {
    name: '韩常青（老韩）',
    title: '首席导师',
    avatar: '/static/avatars/9.png',
    bio: '带你用九型人格看见真实动机，把课程内容落到每天可练习的沟通与成长里。',
    tags: [],
  },
  'ASCII pipes should also split teacher name and identity',
)
assert.equal(
  normalizeTeachers({ home: { teacherTeaser: { title: '韩常青（老韩）', eyebrow: '课程主理人' } } })[0].title,
  '课程主理人',
  'a meaningful eyebrow should provide identity when the title has no pipe',
)
assert.equal(
  normalizeTeachers({ home: { teacherTeaser: { title: '韩常青（老韩）', eyebrow: '老师简介' } } })[0].title,
  '九型人格导师',
  'a generic section eyebrow should not be repeated as teacher identity',
)
assert.equal(
  normalizeTeachers({ home: { teacherTeaser: { title: '韩常青（老韩）', eyebrow: '导师' } } })[0].title,
  '九型人格导师',
  'a generic role label should not replace the useful fallback identity',
)
assert.equal(
  normalizeTeachers({
    teacher: { name: '优先老师', title: '现有来源' },
    home: { teacherTeaser },
  })[0].name,
  '优先老师',
  'existing teacher sources should remain ahead of teacherTeaser',
)

const courseware = normalizeCoursewareItems({
  courseware: { items: [{ name: '九型入门课件', desc: '认识九种核心动机', image: '/cover.png', tag: 'PDF', minutes: '18分钟', link: '/pages/learn/detail' }] },
})[0]
assert.deepEqual(
  courseware,
  {
    title: '九型入门课件',
    description: '认识九种核心动机',
    cover: '/cover.png',
    badge: 'PDF',
    duration: '18分钟',
    materialTypes: ['课件', '音频'],
    bullets: [],
    url: '/pages/learn/detail',
  },
  'courseware.items aliases should be normalized',
)
assert.equal(
  normalizeCoursewareItems({ materials: [{ title: '材料 A' }], lessons: [{ title: '课程 B' }] }).length,
  2,
  'materials and lessons arrays should merge in order',
)
assert.equal(
  normalizeCoursewareItems({ home: { courses: { items: [{ title: '首页课程' }] } } })[0].title,
  '首页课程',
  'existing home.courses.items should remain compatible',
)
assert.equal(
  normalizeCoursewareItems({ courses: { list: [{ title: '课程列表' }] } })[0].title,
  '课程列表',
  'courses.list should be accepted',
)

const realCourses = [
  {
    badge: 'A',
    title: '个人成长 · 关系疗愈',
    description: '认识自己的性格能量，理解反应模式与关系模式，找到成长方向。',
    bullets: ['九型人格与生命关系', '性格情绪与亲子关系', '语言解码'],
  },
  {
    badge: 'B',
    title: '领导力 · 团队协作',
    description: '把九型应用于领导力开发，理解团队成员动机，提升协作与凝聚力。',
    bullets: ['九型人格与领导力', '企业团队训练', '组织文化建设'],
  },
  {
    badge: 'C',
    title: '家庭关系 · 系统排列',
    description: '面向婚姻、亲子、夫妻关系，提供咨询培训与一对一个案咨询治疗经验。',
    bullets: ['夫妻与亲子关系', '家庭系统排列工作坊', '个案疏导'],
  },
]
const enrichedCourses = normalizeCoursewareItems({ home: { courses: { items: realCourses } } })
for (const item of enrichedCourses) {
  assert.deepEqual(Object.keys(item).sort(), courseSchemaKeys, 'backend courses should expose the complete normalized schema')
  assert.equal(Array.isArray(item.bullets), true, 'backend course bullets should always be an array')
  assert.equal(Array.isArray(item.materialTypes), true, 'backend course materialTypes should always be an array')
}
assert.deepEqual(
  enrichedCourses.map((item) => item.bullets),
  realCourses.map((item) => item.bullets),
  'course bullets from the backend should be preserved',
)
assert.deepEqual(
  enrichedCourses.map((item) => item.cover),
  [
    '/static/editorial/course-growth.webp',
    '/static/editorial/course-intro.webp',
    '/static/editorial/course-relation.webp',
  ],
  'the current three course categories should receive distinct local editorial covers',
)
assert.equal(new Set(enrichedCourses.map((item) => item.duration)).size, 3, 'the three categories should receive distinct durations')
assert.equal(new Set(enrichedCourses.map((item) => item.materialTypes.join('/'))).size, 3, 'the three categories should receive distinct material type combinations')

const indexedCourses = normalizeCoursewareItems({
  home: { courses: { items: [{ title: '专题一' }, { title: '专题二' }, { title: '专题三' }] } },
})
assert.deepEqual(
  indexedCourses.map((item) => item.cover),
  enrichedCourses.map((item) => item.cover),
  'unknown course titles should use stable index fallback enrichment',
)

const richCourse = normalizeCoursewareItems({
  home: {
    courses: {
      items: [{
        title: '个人成长专题',
        cover: '/custom-cover.jpg',
        duration: '自定义时长',
        materialTypes: ['直播', '讲义'],
        bullets: ['保留目录'],
      }],
    },
  },
})[0]
assert.equal(richCourse.cover, '/custom-cover.jpg', 'an existing rich cover should win over local enrichment')
assert.equal(richCourse.duration, '自定义时长', 'an existing duration should win over local enrichment')
assert.deepEqual(richCourse.materialTypes, ['直播', '讲义'], 'existing material types should win over local enrichment')
assert.deepEqual(richCourse.bullets, ['保留目录'], 'richer backend course content should remain intact')
assert.deepEqual(Object.keys(richCourse).sort(), courseSchemaKeys, 'backend courseware should use the same normalized schema as defaults')

assert.deepEqual(normalizeCoursewareItems({ home: {} }), DEFAULT_COURSEWARE_ITEMS, 'missing courseware should use stable defaults')
assert.deepEqual(normalizeCoursewareItems({ home: { courses: { items: [] } } }), [], 'an explicit empty home courses section should remain empty')
assert.equal(
  normalizeCoursewareItems({ courseware: [{ title: '优先课件' }], home: { courses: { items: [] } } })[0].title,
  '优先课件',
  'a nonempty higher-priority courseware source should win over an empty home courses section',
)

console.log('teacher/courseware normalization tests passed')
await rm(dir, { force: true, recursive: true })
