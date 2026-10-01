import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const source = await readFile(new URL('./index.vue', import.meta.url), 'utf8')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1]
assert.ok(script, 'home should expose executable page state')
assert.match(source, /class="brand-logo"[^>]+src="\/static\/brand\/logo\.png"/, 'home should use the matching enneagram brand logo')
assert.match(source, /var\(--status-bar-height, env\(safe-area-inset-top, 0px\)\)/, 'home custom navigation should always resolve a valid status-bar fallback')
assert.match(source, /class="studio-header"[^>]+:style="\{ paddingTop: `\$\{statusBarHeight\}px` \}"/, 'the fixed header should own the full status-bar background')
assert.match(source, /\.studio-header\{position:fixed;top:0;/, 'the fixed header background should start at the physical screen top')
assert.match(source, /\.studio-home\{[^}]*padding-top:calc\(var\(--status-bar-height, env\(safe-area-inset-top, 0px\)\) \+ 144rpx\)/, 'fixed navigation should reserve its status bar and header height')
assert.match(source, /class="studio-home"[^>]+:style="\{ paddingTop: `calc\(\$\{statusBarHeight\}px \+ 144rpx\)` \}\"/, 'home should reserve the native status bar height before rendering content')
assert.match(source, /const\s+ORIGINAL_TEACHER_PORTRAIT\s*=\s*['"]\/static\/teacher\/portrait\.jpg['"]/, 'home should retain the original teacher portrait for previews')
assert.match(source, /:src="portraitPreview\s*\|\|\s*portrait"/, 'home preview should use the original portrait source')
assert.match(source, /resolveClassroomItems/, 'home should share the classroom data fallback with the learning page')
const executable = script.replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*;?\s*$/gm, '')
const dir = await mkdtemp(join(tmpdir(), 'nx-studio-home-'))
const modulePath = join(dir, 'home-state.mjs')
await writeFile(modulePath, `
const ref = value => ({ value })
const computed = getter => ({ get value() { return getter() } })
import { resolveHomeNavigation } from '${new URL('../../utils/homeNavigation.js', import.meta.url).href}'
const onMounted = handler => { globalThis.__homeHarness.mount = handler }
const onResize = handler => { globalThis.__homeHarness.resize = handler }
const onShow = handler => { globalThis.__homeHarness.show = handler }
const onShareAppMessage = handler => { globalThis.__homeHarness.share = handler }
const onShareTimeline = handler => { globalThis.__homeHarness.timeline = handler }
const showPublicShareMenu = () => {}
const isTimelinePreview = () => false
const requireFullMiniapp = () => true
const buildShareCard = input => ({ appMessage: input, timeline: input })
const getStoredSiteConfig = () => globalThis.__homeHarness.cache
const refreshSiteConfig = () => globalThis.__homeHarness.refresh()
const listClassroomRecentApi = query => globalThis.__homeHarness.list(query)
const resolveClassroomItems = (remoteItems, bundledItems, options = {}) => {
  if (Array.isArray(remoteItems) && remoteItems.length > 0) return { items: remoteItems, usedFallback: false }
  if (!options.allowFallback) return { items: Array.isArray(remoteItems) ? remoteItems : [], usedFallback: false }
  const fallback = Array.isArray(bundledItems) ? bundledItems : []
  return { items: fallback, usedFallback: fallback.length > 0 }
}
const studioVideos = [{ id: 21, itemType: 'content', title: '本地日常视频', contentType: 'video', coverUrl: '/static/studio-preview/posters/laohan-22.jpg', durationSeconds: 49 }]
const isWechatDevtools = () => globalThis.__homeHarness.devtools === true
const normalizeTeachers = config => config.teachers || (config.home?.teacherTeaser ? [config.home.teacherTeaser] : [])
const normalizeCoursewareItems = config => config.home?.courses?.items || []
const normalizeMiniappCourses = config => {
  const items = config.home?.miniappCourses?.items || normalizeCoursewareItems(config)
  return items.map((item, index) => ({ id: item.id || 'course-' + (index + 1), title: item.title, ...item }))
}
const normalizeMiniappLearn = config => ({ classroom: { enabled: config?.home?.miniappLearn?.classroom?.enabled !== false } })
const classroomContentRoute = item => /^[1-9]\\d*$/.test(String(item?.id || '')) ? '/pages/classroom-detail/classroom-detail?id=' + item.id + '&type=' + (item.contentType === 'audio' ? 'audio' : 'video') : ''
const setBookingIntent = value => { globalThis.__homeHarness.intent = value }
const clearBookingIntent = () => { globalThis.__homeHarness.intent = null }
const STUDIO_TEACHER = { name: '韩老师', avatar: '/static/teacher/portrait.jpg' }
const STUDIO_COURSES = [{ id: 'preview-course', title: '演示课程' }]
const UI_PREVIEW = globalThis.__homeHarness.preview
${executable}
export { config, videos, loading, error, teacher, portrait, portraitPreview, isLaohan, courses, classroomEnabled, dailyVideos, load, daily, booking, openVideo, openCourse, statusBarHeight, topbarStyle }
`)

let counter = 0
async function harness({ preview = false, devtools = false, cache = null, windowInfo = null, systemInfo = null, menuButton = null } = {}) {
  const state = {
    preview, devtools, cache, intent: null, navigations: [], tabs: [],
    refresh: async () => state.cache || {},
    list: async () => ({ items: [] }),
  }
  globalThis.__homeHarness = state
  globalThis.uni = {
    navigateTo: options => state.navigations.push(options),
    switchTab: options => state.tabs.push(options),
    ...(windowInfo === null ? {} : { getWindowInfo: () => windowInfo }),
    ...(systemInfo === null ? {} : { getSystemInfoSync: () => systemInfo }),
    ...(menuButton === null ? {} : { getMenuButtonBoundingClientRect: () => menuButton }),
  }
  const page = await import(`${pathToFileURL(modulePath).href}?case=${++counter}`)
  return { page, state }
}
function deferred() {
  let resolve, reject
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

try {
  {
    const { page } = await harness({ windowInfo: { statusBarHeight: 0 }, systemInfo: { statusBarHeight: 44 } })
    assert.equal(page.statusBarHeight.value, 44, 'a zero modern status-bar value should fall back to the legacy API')
  }
  {
    const { page } = await harness({ windowInfo: { statusBarHeight: 0, safeArea: { top: 44 } }, systemInfo: { statusBarHeight: 0 } })
    assert.equal(page.statusBarHeight.value, 44, 'a zero status-bar API should fall back to the native safe-area top')
  }
  {
    const { page } = await harness()
    assert.equal(page.statusBarHeight.value, 44, 'the custom navigation should keep a visible iPhone fallback in devtools')
  }
  {
    const { page } = await harness({
      windowInfo: { statusBarHeight: 44, windowWidth: 375 },
      menuButton: { left: 278, top: 48 },
    })
    assert.equal(page.topbarStyle.value.paddingRight, '109px', 'the WeChat header should leave room for the capsule')
  }
  {
    const cached = { teachers: [{ name: '已缓存老师' }], home: { courses: { items: [{ title: '正式课程' }] }, miniappLearn: { classroom: { enabled: false } } } }
    const { page } = await harness({ cache: cached })
    assert.equal(page.teacher.value.name, '已缓存老师', 'cached teacher should render before refresh')
    assert.equal(page.classroomEnabled.value, false, 'cached classroom.enabled=false should hide published video entries')
    assert.equal(page.courses.value[0].title, '正式课程', 'production should render configured courses rather than demo schedules')
    assert.equal(page.courses.value[0].id, 'course-1', 'legacy courseware should receive the same normalized ID used by the detail page')
    assert.deepEqual(page.videos.value, [], 'production should not invent a published video list')
  }
  {
    const { page } = await harness({ preview: true })
    assert.equal(page.courses.value[0].id, 'preview-course', 'explicit preview may display the isolated sample schedule')
  }
  {
    const { page, state } = await harness({ cache: {
      home: { miniappCourses: { items: [{ id: 'paid-intro', title: '付费入门课', paymentMode: 'paid', priceCents: 19800 }] } },
    } })
    assert.equal(page.courses.value[0].id, 'paid-intro', 'home should use the managed mini-app course catalog')
    page.openCourse()
    assert.equal(state.navigations.at(-1).url, '/pages/course-detail/course-detail?id=paid-intro', 'home should pass the managed course ID to detail')
  }
  {
    const { page, state } = await harness({ cache: { teachers: [{ name: '原老师' }] } })
    state.refresh = async () => {
      state.cache = { teachers: [{ name: '更新老师' }] }
      return state.cache
    }
    state.list = async query => {
      assert.equal(query.limit, 6)
      return { items: [{ id: 8, itemType: 'series' }, { id: 21, title: '第一讲' }, { id: 20, title: '第二讲' }, { id: 19, title: '第三讲' }] }
    }
    await page.load()
    assert.equal(page.teacher.value.name, '更新老师')
    assert.deepEqual(page.dailyVideos.value.map(item => item.id), [21, 20], 'home should show two actual content entries, not misroute series as videos')
    assert.equal(page.loading.value, false)
    assert.equal(page.error.value, '')
  }
  {
    const { page, state } = await harness({ devtools: true })
    state.list = async () => ({ items: [] })
    await page.load()
    assert.deepEqual(page.dailyVideos.value.map(item => item.id), [21], 'developer tools should keep bundled daily videos visible when the API response is empty')
    assert.equal(page.dailyVideos.value[0].coverUrl, '/static/studio-preview/posters/laohan-22.jpg')
    assert.equal(page.error.value, '', 'bundled daily videos should clear the transient API error')
  }
  {
    const { page, state } = await harness({ devtools: true })
    state.list = async () => { throw new Error('network offline') }
    await page.load()
    assert.deepEqual(page.dailyVideos.value.map(item => item.id), [21], 'developer tools should keep bundled daily videos visible when the API request fails')
    assert.equal(page.error.value, '', 'bundled daily videos should replace the transient API error')
  }
  {
    const { page, state } = await harness({ cache: { teachers: [{ name: '保留老师' }] } })
    // siteConfig cache tests verify the merge itself; this checks that the page
    // reads the merged cache instead of replacing it with the partial payload.
    state.refresh = async () => {
      state.cache = { teachers: [{ name: '保留老师' }], home: { miniappLearn: { classroom: { enabled: false } } } }
      return { home: { miniappLearn: { classroom: { enabled: false } } } }
    }
    await page.load()
    assert.equal(page.teacher.value.name, '保留老师', 'partial config refresh must retain the previously configured teacher')
    assert.equal(page.classroomEnabled.value, false, 'partial refresh should still update the classroom visibility switch')
    state.refresh = async () => { state.cache = { teachers: [] }; return state.cache }
    await page.load()
    assert.equal(page.teacher.value, null, 'an explicitly empty teacher section must hide the teacher instead of resurrecting fallback content')
  }
  {
    const { page, state } = await harness({ cache: { teachers: [{ name: '缓存老师' }] } })
    state.refresh = async () => { throw new Error('config offline') }
    state.list = async () => ({ items: [{ id: 21 }] })
    await page.load()
    assert.equal(page.teacher.value.name, '缓存老师', 'a config failure should preserve cached teacher content')
    assert.deepEqual(page.dailyVideos.value.map(item => item.id), [21], 'a config failure should not block independent video content')
    state.refresh = async () => { state.cache = { teachers: [{ name: '新老师' }] }; return state.cache }
    state.list = async () => { throw new Error('video offline') }
    await page.load()
    assert.equal(page.teacher.value.name, '新老师', 'a video failure should not block teacher updates')
    assert.deepEqual(page.videos.value.map(item => item.id), [21], 'a video failure should retain already fetched video data')
    assert.ok(page.error.value, 'a video failure must stay distinguishable from an empty library')
    assert.equal(page.loading.value, false, 'request failure should always release the loading state')
  }
  {
    const { page, state } = await harness()
    const oldConfig = deferred(), oldVideos = deferred(), newConfig = deferred(), newVideos = deferred()
    let configCount = 0, videoCount = 0
    state.refresh = () => (++configCount === 1 ? oldConfig.promise : newConfig.promise)
    state.list = () => (++videoCount === 1 ? oldVideos.promise : newVideos.promise)
    const older = page.load(), newer = page.load()
    state.cache = { teachers: [{ name: '最新老师' }] }
    newConfig.resolve(state.cache)
    newVideos.resolve({ items: [{ id: 22 }] })
    await newer
    oldConfig.resolve({ teachers: [{ name: '过期老师' }] })
    oldVideos.resolve({ items: [{ id: 11 }] })
    await older
    assert.equal(page.teacher.value.name, '最新老师', 'a late response must not replace current teacher content')
    assert.deepEqual(page.videos.value.map(item => item.id), [22], 'a late response must not replace current video content')
  }
  {
    const { page, state } = await harness()
    page.daily()
    assert.equal(state.tabs.at(-1).url, '/pages/learn/learn')
    page.booking()
    assert.deepEqual(state.intent, { kind: 'course', intentText: '' }, 'course entry should select course booking')
    assert.equal(state.tabs.at(-1).url, '/pages/booking/booking')
    page.booking('consult')
    assert.deepEqual(state.intent, { kind: 'consult', intentText: '' }, 'consultation entry should select consultation booking')
    state.tabs.at(-1).fail()
    assert.equal(state.intent, null, 'failed tab navigation must clear booking intent')
    page.openVideo({ id: 21, contentType: 'audio' })
    assert.equal(state.navigations.at(-1).url, '/pages/classroom-detail/classroom-detail?id=21&type=audio')
    const count = state.navigations.length
    page.openVideo({ id: 'invalid' })
    assert.equal(state.navigations.length, count, 'invalid content IDs must not produce broken detail routes')
  }
  {
    const { page } = await harness({ cache: { teachers: [{ name: '李老师', avatar: '/static/avatars/9.png' }] } })
    assert.equal(page.isLaohan.value, false, 'a configured teacher must not inherit another teacher identity')
    assert.equal(page.portrait.value, '', 'missing portraits for another teacher must not display Laohan')
    page.config.value = { teachers: [{ name: '韩梅', avatar: '/static/avatars/9.png' }] }
    assert.equal(page.isLaohan.value, false, 'a shared surname does not establish teacher identity')
    page.config.value = { teachers: [{ name: '韩常青（老韩）', avatar: 'https://site.example/assets/teacher-poster.jpg' }] }
    assert.equal(page.isLaohan.value, true)
    assert.equal(page.portrait.value, '/static/teacher/hero-portrait.jpg', 'verified Laohan uses the clean portrait instead of cropping a text poster')
    page.config.value = { teachers: [{ name: '韩常青', avatar: '/static/teacher/portrait.jpg' }] }
    assert.equal(page.portrait.value, '/static/teacher/hero-portrait.jpg', 'home should use the wide teacher composition for the portrait avatar')
    assert.equal(page.portraitPreview.value, '/static/teacher/portrait.jpg', 'home preview should keep the original portrait instead of the wide composition')
  }
  console.log('teacher studio home state tests passed')
} finally {
  delete globalThis.__homeHarness
  delete globalThis.uni
  await rm(dir, { recursive: true, force: true })
}
