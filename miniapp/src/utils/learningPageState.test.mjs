import assert from 'node:assert/strict'
import {
  applyLearningContent,
  createActionActivationGuard,
  createInitialLearningContent,
  createLatestRequestGuard,
  createOneShotFallbackRegistry,
  handleActionKeydown,
  retainLearningContentOnError,
} from './learningPageState.js'

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

console.log('learning page state tests passed')
