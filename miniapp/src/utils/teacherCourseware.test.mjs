import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-teacher-courseware-'))
const modulePath = join(dir, 'teacherCourseware.mjs')
const contentAssetPath = join(dir, 'contentAsset.mjs')
await writeFile(
  contentAssetPath,
  (await readFile(new URL('./contentAsset.js', import.meta.url), 'utf8'))
    .replace(
      /import \{ API_BASE(?:, DEFAULT_API_BASE)? \} from '\.\.\/config'/,
      "const API_BASE = 'https://api.example.test/api'; const DEFAULT_API_BASE = API_BASE",
    ),
)
let source = (await readFile(new URL('./teacherCourseware.js', import.meta.url), 'utf8'))
  .replace("'./contentAsset.js'", "'./contentAsset.mjs'")
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
  normalizeTeachers({ teacher: { name: '韩老师', title: '九型导师', avatar: '/static/a.png', bio: '十年咨询经验', tags: ['课程研发'] } })[0],
  { name: '韩老师', title: '九型导师', avatar: '/static/a.png', bio: '十年咨询经验', tags: ['课程研发'] },
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

assert.deepEqual(
  normalizeTeachers({
    home: {
      teacherTeaser: {
        eyebrow: '九型人格导师',
        title: '韩老师',
        image: '/assets/teacher-poster.jpg',
        fallbackImage: '/assets/teacher.svg',
        lead: '带你把课程练习落到生活。',
      },
    },
  })[0],
  {
    name: '韩老师',
    title: '九型人格导师',
    avatar: 'https://api.example.test/assets/teacher-poster.jpg',
    bio: '带你把课程练习落到生活。',
    tags: [],
  },
  'legacy home.teacherTeaser should map into the structured teacher view model',
)
assert.equal(
  normalizeTeachers({ home: { teacherTeaser: { fallbackImage: '/assets/teacher.svg', lead: '简介' } } })[0].name,
  '九型老师',
  'legacy teaser without a title should use the teacher-name fallback',
)
assert.equal(
  normalizeTeachers({ home: { teacherTeaser: { fallbackImage: '/assets/teacher.svg', lead: '简介' } } })[0].avatar,
  'https://api.example.test/assets/teacher.svg',
  'legacy fallbackImage should supply the avatar when image is absent',
)
assert.equal(
  normalizeTeachers({
    home: { teacherTeaser: { image: '/legacy.png', fallbackImage: '/assets/teacher.svg', lead: '简介' } },
  })[0].avatar,
  'https://api.example.test/assets/teacher.svg',
  'an unsupported teaser image should continue to the valid fallbackImage',
)
assert.deepEqual(
  normalizeTeachers({
    teacher: { name: '结构化老师', title: '结构化职称', avatar: '/static/structured.png', bio: '结构化简介' },
    home: { teacherTeaser: { title: '旧老师', eyebrow: '旧职称', image: '/legacy.png', lead: '旧简介' } },
  }),
  [{ name: '结构化老师', title: '结构化职称', avatar: '/static/structured.png', bio: '结构化简介', tags: [] }],
  'structured teacher data should be authoritative when legacy teaser data also exists',
)
for (const structured of [
  { teacher: {} },
  { teacher: { name: '  ', title: '\n', avatar: '', bio: ' ' } },
  { teachers: [] },
  { teachers: [{}] },
]) {
  assert.equal(
    normalizeTeachers({
      ...structured,
      home: { teacherTeaser: { title: '有效 teaser 老师', eyebrow: '导师', image: '/teaser.png', lead: '有效简介' } },
    })[0].name,
    '有效 teaser 老师',
    'empty or blank structured teacher entries must not hide a valid legacy teaser',
  )
}
assert.deepEqual(
  normalizeTeachers({
    teachers: [{}, { name: '  ' }, { teacherName: '有效结构化老师', desc: '结构化简介' }],
    home: { teacherTeaser: { title: '旧 teaser 老师', lead: '旧简介' } },
  }).map((teacher) => teacher.name),
  ['有效结构化老师'],
  'empty structured entries should be filtered while a real structured teacher remains authoritative',
)

const courseware = normalizeCoursewareItems({
  courseware: { items: [{ name: '九型入门课件', desc: '认识九种核心动机', image: '/assets/course-cover.jpg', tag: 'PDF', minutes: '18分钟', link: '/pages/learn/detail' }] },
})[0]
assert.deepEqual(
  courseware,
  {
    title: '九型入门课件',
    description: '认识九种核心动机',
    cover: 'https://api.example.test/assets/course-cover.jpg',
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
  normalizeCoursewareItems({
    home: { courses: { items: [{ title: '课程产品' }] } },
    classroom: { contents: [{ title: '独立视频课件', contentType: 'video' }] },
  })[0].title,
  '课程产品',
  'independent classroom media must not overwrite legacy course products',
)
assert.deepEqual(
  normalizeCoursewareItems({ classroom: { contents: [{ title: '独立音频课件', contentType: 'audio' }] } }),
  DEFAULT_COURSEWARE_ITEMS,
  'classroom media alone should not be normalized as legacy course products',
)
assert.equal(
  normalizeCoursewareItems({ courses: { list: [{ title: '课程列表' }] } })[0].title,
  '课程列表',
  'courses.list should be accepted',
)
assert.deepEqual(normalizeCoursewareItems({ home: {} }), DEFAULT_COURSEWARE_ITEMS, 'missing courseware should use stable defaults')

console.log('teacher/courseware normalization tests passed')
await rm(dir, { force: true, recursive: true })
