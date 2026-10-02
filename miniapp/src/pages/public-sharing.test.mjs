import { isCourseRegistrationEnabled } from '../utils/courseRegistration.js'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'

function harness(name, options = {}) {
  const source = readFileSync(new URL(`./${name}/${name}.vue`, import.meta.url), 'utf8')
  const script = source.match(/<script setup>([\s\S]*?)<\/script>/)[1].replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*;?\s*$/gm, '')
  const hooks = { hints: 0, navigations: 0, show: [], mount: [], menus: [], cards: [], privateQueries: [], login: 0 }
  const state = { config: options.config || { teachers: [{ name: '当前老师', title: '成长导师', avatar: '/static/current-teacher.jpg' }], courses: [{ id: 'growth-lesson', title: '真实成长课', cover: '/static/course-current.jpg' }] } }
  const context = vm.createContext({
    isCourseRegistrationEnabled,
    ref: value => ({ value }), computed: getter => ({ get value() { return getter() } }),
    onHide: fn => { hooks.hide = fn }, onLoad: fn => { hooks.load = fn }, onUnload: fn => { hooks.unload = fn }, onResize: () => {},
    onMounted: fn => hooks.mount.push(fn), onShow: fn => hooks.show.push(fn),
    onShareAppMessage: fn => { hooks.friend = fn }, onShareTimeline: fn => { hooks.timeline = fn },
    buildShareCard: input => { hooks.cards.push(input); return { appMessage: { ...input }, timeline: { ...input } } },
    isTimelinePreview: () => options.timeline === true, requireFullMiniapp: () => { if (options.timeline) { hooks.hints++; return false }; return true },
    showPublicShareMenu: enabled => hooks.menus.push(enabled === undefined ? true : enabled),
    getStoredSiteConfig: () => state.config, refreshSiteConfig: async () => { await options.loadGate; return state.config },
    getCachedSiteConfig: async () => { await options.loadGate; return state.config },
    normalizeTeachers: config => config.teachers || [], normalizeMiniappCourses: config => config.courses || [],
    normalizeMiniappLearn: () => ({ classroom: { enabled: true } }),
    setBookingIntent: () => true,
    getToken: () => options.token || '', ensureLogin: async () => { hooks.login++ },
    getCourseEnrollmentApi: async query => { hooks.privateQueries.push(query); return options.enrollment || { courseId: query.courseId, owned: false, syncStatus: 'confirmed', bookingId: '', order: null } },
    resolveHomeNavigation: () => ({ statusBarHeight: 44 }),
    createInitialLearningContent: () => ({ teachers: state.config.teachers, quotes: [] }),
    createOneShotFallbackRegistry: () => ({}), createLatestRequestGuard: () => ({}), createActionActivationGuard: () => ({}),
    readLearningNavIntent: () => null, resolveLearningCategory: () => 'course',
    createLearningCourseEntries: () => [], createLearningQuoteEntries: () => [], createLearningTagEntries: () => [],
    useStudioNavigation: () => ({ pageStyle: {}, refreshNavigation: () => {} }),
    TYPES_INFO: {}, studioVideos: [], UI_PREVIEW: false, STUDIO_TEACHER: {}, STUDIO_COURSES: [],
    uni: { navigateTo: () => { hooks.navigations++ }, switchTab: () => { hooks.navigations++ } },
  })
  const exposed = name === 'learn' ? 'teachers, teacherImage, openTypeDetail, openPublishedCourse, openPublishedMaterial' : name === 'course-detail' ? 'config, courseId, enrollment, loading, enroll, teacherDetail, openMyCourse, goBack' : name === 'teacher' ? 'config, book, daily' : 'config, navigate, booking, daily'
  vm.runInContext(`${script}\nglobalThis.page = { ${exposed} }`, context)
  return { source, hooks, state, page: context.page }
}
for (const [name, kind] of [['index', 'home'], ['teacher', 'teacher'], ['learn', 'daily']]) {
  const { source, hooks, page } = harness(name)
  assert.equal(typeof hooks.friend, 'function', `${name} registers native friend sharing on the page`)
  assert.equal(typeof hooks.timeline, 'function', `${name} registers native Timeline sharing on the page`)
  assert.match(source, /<NxShareActions\b/, `${name} exposes discoverable share actions`)
  await Promise.all(hooks.show.map(fn => fn()))
  assert.equal(hooks.menus.at(-1), true, `${name} enables both menus when revisited`)
  assert.equal(hooks.friend().kind, kind)
  assert.match(hooks.friend().title, /当前老师/, `${name} uses current public teacher details`)
  if (name === 'learn') {
    page.teachers.value = [{ name: '更新老师', title: '关系导师' }]
    page.teacherImage.value = '/static/updated-teacher.jpg'
  } else page.config.value = { teachers: [{ name: '更新老师', title: '关系导师', avatar: '/static/updated-teacher.jpg' }] }
  const next = hooks.timeline()
  assert.equal(next.kind, kind)
  assert.match(next.title, /更新老师/, `${name} resolves current data at share time`)
  assert.equal(next.imageUrl, '/static/updated-teacher.jpg')
  assert.ok(!('bookingId' in next) && !('token' in next))
}
{
  let finish
  const { source, hooks, page } = harness('course-detail', { loadGate: new Promise(resolve => { finish = resolve }) })
  assert.equal(typeof hooks.friend, 'function', 'course registers the native friend callback')
  assert.equal(typeof hooks.timeline, 'function', 'course registers the native Timeline callback')
  assert.match(source, /<NxShareActions[^>]*:disabled="!courseShareable"/, 'unavailable course sharing has a disabled button')
  const loading = hooks.load({ id: 'growth-lesson', bookingId: 'secret-booking', paid: 'true' })
  const showing = Promise.all(hooks.show.map(fn => fn()))
  assert.equal(hooks.menus.at(-1), false, 'course sharing remains disabled while public catalog loads')
  finish()
  await Promise.all([loading, showing])
  assert.equal(hooks.menus.at(-1), true)
  assert.equal(hooks.login, 0, 'cold public course entry does not force login')
  assert.equal(hooks.privateQueries.length, 0, 'guest share recipient never fetches another account enrollment')
  const friend = hooks.friend()
  assert.equal(friend.kind, 'course')
  assert.equal(friend.id, 'growth-lesson')
  assert.equal(friend.title, '真实成长课')
  assert.equal(friend.imageUrl, '/static/course-current.jpg')
  assert.deepEqual(Object.keys(friend).sort(), ['id', 'imageUrl', 'kind', 'title'])
  page.config.value.courses[0].title = '后台更新的课程'
  assert.equal(hooks.timeline().title, '后台更新的课程')
  page.config.value.courses = []
  page.enrollment.value = { owned: true, order: { status: 'paid', title: '个人订单标题', bookingId: 'secret-booking' } }
  await Promise.all(hooks.show.map(fn => fn()))
  assert.equal(hooks.menus.at(-1), false, 'private enrollment fallback does not enable public sharing')
  assert.equal(hooks.friend().kind, 'home', 'a stale native share event gets a public fallback without page query data')
  page.config.value.courses = [{ id: 'bad?id=private', title: '无效课程' }]
  page.courseId.value = 'bad?id=private'
  assert.equal(hooks.timeline().kind, 'home', 'invalid identifiers cannot form share links')
}
{
  let finish
  const { hooks } = harness('course-detail', { loadGate: new Promise(resolve => { finish = resolve }) })
  const loading = hooks.load({ id: 'growth-lesson' })
  const showing = Promise.all(hooks.show.map(fn => fn()))
  assert.equal(typeof hooks.hide, 'function')
  hooks.hide()
  const menuUpdates = hooks.menus.length
  finish()
  await Promise.all([loading, showing])
  assert.equal(hooks.menus.length, menuUpdates, 'a hidden course page never reopens menus over the active private page')
  await Promise.all(hooks.show.map(fn => fn()))
  assert.equal(hooks.menus.at(-1), true, 'returning to the loaded public course restores sharing')
}
for (const name of ['index', 'teacher', 'learn', 'course-detail']) {
  const { hooks, page } = harness(name, { timeline: true, token: 'previous-session' })
  if (hooks.load) await hooks.load({ id: 'growth-lesson', paid: 'true' })
  await Promise.all(hooks.show.map(fn => fn()))
  if (name === 'index') { page.navigate('/private'); page.booking(); page.daily() }
  if (name === 'teacher') { page.book(); page.daily() }
  if (name === 'learn') { page.openTypeDetail(1); page.openPublishedCourse({ id: 21 }); page.openPublishedMaterial({ id: 21 }) }
  if (name === 'course-detail') { await page.enroll(); page.openMyCourse(); page.teacherDetail(); page.goBack() }
  assert.ok(hooks.hints > 0, `${name} directs single-page readers to the full miniapp`)
  assert.equal(hooks.navigations, 0, `${name} never invokes unsupported Timeline navigation APIs`)
  assert.equal(hooks.login, 0)
  assert.equal(hooks.privateQueries.length, 0, 'Timeline preview never reads private enrollment even with stale storage')
}
console.log('public page friend and Timeline sharing tests passed')

{
  const { hooks } = harness('course-detail', { config: { home: { miniappCourses: { enabled: false } }, courses: [{ id: 'growth-lesson', title: '隐藏课程' }] } })
  await hooks.load({ id: 'growth-lesson' })
  assert.equal(hooks.menus.at(-1), false)
  assert.equal(hooks.friend().kind, 'home', 'closed course links must not share a sales card')
  assert.equal(hooks.navigations, 1, 'old public links redirect to the real booking form')
}
