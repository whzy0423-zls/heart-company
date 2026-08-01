import assert from 'node:assert/strict'
import {
  applyLearningContent,
  createActionActivationGuard,
  createInitialLearningContent,
  createLatestRequestGuard,
  createLearningCourseEntries,
  createLearningQuoteEntries,
  createLearningTagEntries,
  createOneShotFallbackRegistry,
  flattenLearningMaterials,
  handleActionKeydown,
  learningTabTransition,
  resolveLearningCategory,
  retainLearningContentOnError,
} from './learningPageState.js'
import {
  LEARNING_NAV_INTENT_KEY,
  readLearningNavIntent,
  setLearningNavIntent,
} from './learningNavIntent.js'

const initial = createInitialLearningContent()
assert.ok(initial.teachers.length > 0, 'uncached learning content should start with the local teacher fallback')
assert.ok(initial.coursewareItems.length > 0, 'uncached learning content should start with local course fallbacks')
assert.deepEqual(initial.quotes, [], 'uncached learning content should start without invented quotes')

const current = {
  teachers: [{ name: '缓存老师' }],
  coursewareItems: [{ title: '缓存课程' }],
  quotes: ['缓存语录'],
}
assert.deepEqual(
  applyLearningContent(current, { navigation: { title: 'partial refresh' } }, { preserveMissing: true }),
  current,
  'a successful partial refresh should preserve learning sections that are missing',
)

const explicitEmpty = applyLearningContent(current, {
  teachers: [],
  courseware: [],
  home: { quotes: { items: [] } },
}, { preserveMissing: true })
assert.deepEqual(explicitEmpty.teachers, [], 'an explicitly empty teacher section should replace cached teachers')
assert.deepEqual(explicitEmpty.coursewareItems, [], 'an explicitly empty course section should replace cached courses')
assert.deepEqual(explicitEmpty.quotes, [], 'an explicitly empty quote section should replace cached quotes')

const cached = applyLearningContent(initial, {
  home: {
    teacherTeaser: { name: '后台老师', title: '课程导师' },
    courses: [{ title: '后台课程', description: '课程介绍' }],
    quotes: { items: ['后台语录', '后台语录'] },
  },
})
assert.equal(cached.teachers[0].name, '后台老师', 'cached teacher content should replace the local fallback immediately')
assert.equal(cached.coursewareItems[0].title, '后台课程', 'cached course content should replace local course fallbacks immediately')
assert.deepEqual(cached.quotes, ['后台语录', '后台语录'], 'duplicate backend display values should remain available for indexed rendering')

const failed = retainLearningContentOnError(cached, '网络失败')
assert.equal(failed.teachers, cached.teachers, 'request failure should retain the current teacher content')
assert.equal(failed.coursewareItems, cached.coursewareItems, 'request failure should retain the current course content')
assert.equal(failed.quotes, cached.quotes, 'request failure should retain the current quote content')
assert.equal(failed.loadError, '网络失败', 'request failure should attach a nonblocking error message')

const requests = createLatestRequestGuard()
const firstTicket = requests.issue()
const secondTicket = requests.issue()
assert.equal(requests.isLatest(firstTicket), false, 'an out-of-order older response should be ignored')
assert.equal(requests.isLatest(secondTicket), true, 'the latest response should be accepted')

const fallbacks = createOneShotFallbackRegistry()
assert.equal(fallbacks.consume('teacher'), true, 'the first image error should apply its fallback')
assert.equal(fallbacks.consume('teacher'), false, 'a repeated image error should not loop the same fallback')
assert.equal(fallbacks.consume('course:0'), true, 'fallback usage should be tracked separately per image')
fallbacks.reset()
assert.equal(fallbacks.consume('teacher'), true, 'new content should reset one-shot fallback tracking')

let now = 1000
const activation = createActionActivationGuard({ now: () => now })
const target = {}
assert.equal(activation.shouldActivate({ type: 'keydown', key: 'Enter', currentTarget: target }), true)
assert.equal(activation.shouldActivate({ type: 'keydown', key: 'Enter', repeat: true, currentTarget: target }), false, 'repeated keydown should not activate')
now = 1100
assert.equal(activation.shouldActivate({ type: 'click', currentTarget: target }), false, 'synthetic click following keyboard activation should be suppressed')
now = 1700
assert.equal(activation.shouldActivate({ type: 'click', currentTarget: target }), true, 'a later pointer click should activate normally')

let prevented = 0
let stopped = 0
let activated = 0
const tabHandled = handleActionKeydown({
  key: 'Tab',
  preventDefault: () => { prevented += 1 },
  stopPropagation: () => { stopped += 1 },
}, () => { activated += 1 })
assert.equal(tabHandled, false, 'Tab should not be treated as an activation key')
assert.equal(prevented, 0, 'Tab should not be prevented')
assert.equal(stopped, 0, 'Tab propagation should remain untouched')
assert.equal(activated, 0, 'Tab should not trigger the action')

const enterHandled = handleActionKeydown({
  key: 'Enter',
  preventDefault: () => { prevented += 1 },
  stopPropagation: () => { stopped += 1 },
}, () => { activated += 1 })
assert.equal(enterHandled, true, 'Enter should be handled as an activation key')
assert.equal(prevented, 1, 'Enter should be prevented after key filtering')
assert.equal(stopped, 1, 'handled activation keys should stop propagation')
assert.equal(activated, 1, 'Enter should trigger the supplied action')

const materialCourses = [
  {
    id: 'course-a',
    title: '九型入门',
    description: '建立九型基础地图',
    duration: '20 分钟',
    materialTypes: [
      { id: 'slides', name: '讲义' },
      { id: 'audio', name: '音频' },
    ],
  },
  {
    id: 'course-b',
    title: '关系练习',
    materialTypes: ['讲义', '讲义'],
  },
]
const flattenedMaterials = flattenLearningMaterials(materialCourses)
assert.deepEqual(
  flattenedMaterials.map(({ courseTitle, type }) => ({ courseTitle, type })),
  [
    { courseTitle: '九型入门', type: '讲义' },
    { courseTitle: '九型入门', type: '音频' },
    { courseTitle: '关系练习', type: '讲义' },
    { courseTitle: '关系练习', type: '讲义' },
  ],
  'course data should flatten into stable, unique material entries using course and material identities',
)
assert.equal(new Set(flattenedMaterials.map((item) => item.key)).size, flattenedMaterials.length, 'flattened material keys should remain unique')
assert.deepEqual(
  flattenLearningMaterials(materialCourses).map((item) => item.key),
  flattenedMaterials.map((item) => item.key),
  'material keys should remain stable across repeated normalization',
)
const sameTitleCourses = [
  { title: '沟通练习', description: '亲密关系沟通', duration: '30 分钟', materialTypes: ['讲义'] },
  { title: '沟通练习', description: '团队协作沟通', duration: '45 分钟', materialTypes: ['讲义'] },
]
const sameTitleKeys = (courses) => Object.fromEntries(
  flattenLearningMaterials(courses).map((item) => [item.description, item.key]),
)
assert.deepEqual(
  sameTitleKeys([...sameTitleCourses].reverse()),
  sameTitleKeys(sameTitleCourses),
  'same-title courses with distinguishable stable content should retain their logical material keys when backend order changes',
)

const sharedExplicitIdentity = [
  {
    id: 'shared-course',
    title: '同名课程',
    description: '课程甲',
    materialTypes: [{ id: 'shared-material', name: '讲义', url: '/a.pdf' }],
  },
  {
    id: 'shared-course',
    title: '同名课程',
    description: '课程乙',
    materialTypes: [{ id: 'shared-material', name: '讲义', url: '/b.pdf' }],
  },
]
assert.deepEqual(
  sameTitleKeys([...sharedExplicitIdentity].reverse()),
  sameTitleKeys(sharedExplicitIdentity),
  'different courses and materials sharing explicit ids should retain deterministic keys under reorder',
)

const priorFnvCollisionCourses = [
  { title: '哈希碰撞课程', description: 'desc-7iilspgbrvu-91242', materialTypes: ['讲义'] },
  { title: '哈希碰撞课程', description: 'desc-2mqk7cq64l7-132603', materialTypes: ['讲义'] },
]
assert.deepEqual(
  sameTitleKeys([...priorFnvCollisionCourses].reverse()),
  sameTitleKeys(priorFnvCollisionCourses),
  'known prior FNV collision content should not fall back to encounter-order suffixes',
)

const circularCourse = { title: '循环课程', description: '循环结构也不能导致页面失败' }
const circularMaterial = { name: '循环讲义' }
circularCourse.self = circularCourse
circularMaterial.course = circularCourse
circularCourse.materialTypes = [circularMaterial]
assert.doesNotThrow(() => flattenLearningMaterials([circularCourse]), 'circular course and material data should be canonicalized safely')
assert.equal(flattenLearningMaterials([circularCourse]).length, 1, 'valid circular material content should remain renderable')

const throwingCourse = { title: '异常课程' }
Object.defineProperty(throwingCourse, 'materialTypes', { get() { throw new Error('getter failed') } })
const throwingMaterial = { id: 'bad-material' }
Object.defineProperty(throwingMaterial, 'name', { get() { throw new Error('getter failed') } })
assert.doesNotThrow(
  () => flattenLearningMaterials([throwingCourse, { title: '部分异常', materialTypes: [throwingMaterial, '正常讲义'] }]),
  'throwing accessors should fail closed instead of blanking the learning page',
)
assert.equal(
  flattenLearningMaterials([throwingCourse, { title: '部分异常', materialTypes: [throwingMaterial, '正常讲义'] }]).length,
  1,
  'malformed entries should be skipped while valid sibling materials remain available',
)
const exoticMaterial = {
  name: '特殊类型讲义',
  date: new Date('2026-07-22T00:00:00.000Z'),
  map: new Map([['b', 2], ['a', 1]]),
  set: new Set(['b', 'a']),
  callable: function lessonHelper() {},
  symbolic: Symbol.for('lesson'),
  large: 9007199254740993n,
  missing: undefined,
  nan: NaN,
  positiveInfinity: Infinity,
  negativeInfinity: -Infinity,
}
const exoticCourse = { title: '特殊类型课程', materialTypes: [exoticMaterial] }
assert.doesNotThrow(() => flattenLearningMaterials([exoticCourse]), 'supported non-JSON values should have deterministic fail-closed identities')
assert.equal(
  flattenLearningMaterials([exoticCourse])[0].key,
  flattenLearningMaterials([exoticCourse])[0].key,
  'Date, Map, Set, function, symbol, bigint, undefined, and non-finite number handling should be deterministic',
)
const throwingProxyMaterial = new Proxy({ name: '代理讲义' }, { ownKeys() { throw new Error('ownKeys failed') } })
assert.doesNotThrow(
  () => flattenLearningMaterials([{ title: '代理课程', materialTypes: [throwingProxyMaterial] }]),
  'property enumeration failures should be contained by canonicalization',
)

const courseEntries = createLearningCourseEntries(sameTitleCourses)
assert.equal(new Set(courseEntries.map((entry) => entry.key)).size, courseEntries.length, 'course rows should receive stable unique keys')
assert.deepEqual(
  Object.fromEntries(createLearningCourseEntries([...sameTitleCourses].reverse()).map((entry) => [entry.item.description, entry.key])),
  Object.fromEntries(courseEntries.map((entry) => [entry.item.description, entry.key])),
  'course row keys should follow logical content rather than array position',
)
const quoteEntries = createLearningQuoteEntries(['同一句', '另一句', '同一句'])
assert.equal(new Set(quoteEntries.map((entry) => entry.key)).size, quoteEntries.length, 'duplicate quotes should still receive unique row keys')
assert.equal(
  createLearningQuoteEntries(['另一句', '同一句']).find((entry) => entry.text === '另一句').key,
  createLearningQuoteEntries(['同一句', '另一句']).find((entry) => entry.text === '另一句').key,
  'distinct quote keys should not depend on list position',
)
const tagEntries = createLearningTagEntries(['关系沟通', '成长练习', '关系沟通'], 'teacher-a')
assert.equal(new Set(tagEntries.map((entry) => entry.key)).size, tagEntries.length, 'duplicate teacher tags should receive unique stable keys')

assert.deepEqual(learningTabTransition('course', 'Enter'), { handled: true, category: 'course', focusIndex: 0 }, 'Enter should select the focused tab')
assert.deepEqual(learningTabTransition('material', ' '), { handled: true, category: 'material', focusIndex: 1 }, 'Space should select the focused tab')
assert.deepEqual(learningTabTransition('course', 'ArrowLeft'), { handled: true, category: 'quote', focusIndex: 2 }, 'ArrowLeft should wrap to the last tab')
assert.deepEqual(learningTabTransition('quote', 'ArrowRight'), { handled: true, category: 'course', focusIndex: 0 }, 'ArrowRight should wrap to the first tab')
assert.deepEqual(learningTabTransition('material', 'Tab'), { handled: false, category: 'material', focusIndex: 1 }, 'unhandled keys should retain selection and focus')
assert.deepEqual(flattenLearningMaterials(null), [], 'malformed course data should flatten safely')
assert.deepEqual(
  flattenLearningMaterials([{ title: '无资料课程', materialTypes: null }, null]),
  [],
  'courses without valid material types should not create placeholder materials',
)

for (const category of ['course', 'material', 'quote']) {
  assert.equal(resolveLearningCategory('course', category), category, `${category} navigation intent should select its matching category`)
}
assert.equal(resolveLearningCategory('material', null), 'material', 'no navigation intent should retain the current valid category')
assert.equal(resolveLearningCategory('quote', 'unknown'), 'quote', 'invalid navigation intent should retain the current valid category')
assert.equal(resolveLearningCategory('unknown', null), 'course', 'an invalid current category should fall back to courses')

const intentStorage = new Map()
globalThis.uni = {
  getStorageSync: (key) => intentStorage.has(key) ? intentStorage.get(key) : '',
  setStorageSync: (key, value) => intentStorage.set(key, value),
  removeStorageSync: (key) => intentStorage.delete(key),
}
setLearningNavIntent('quote', { now: () => 1000 })
const categoryAfterShow = resolveLearningCategory('course', readLearningNavIntent({ now: () => 1001 }))
assert.equal(categoryAfterShow, 'quote', 'onShow-style intent consumption should change to the requested category')
assert.equal(intentStorage.has(LEARNING_NAV_INTENT_KEY), false, 'reading a navigation intent should clear it immediately')
assert.equal(
  resolveLearningCategory(categoryAfterShow, readLearningNavIntent({ now: () => 1002 })),
  'quote',
  'a later onShow without an intent should retain the current category',
)
delete globalThis.uni

console.log('learning page state tests passed')
