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
assert.deepEqual(normalizeCoursewareItems({ home: {} }), DEFAULT_COURSEWARE_ITEMS, 'missing courseware should use stable defaults')

console.log('teacher/courseware normalization tests passed')
await rm(dir, { force: true, recursive: true })
