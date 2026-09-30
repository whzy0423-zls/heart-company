import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const source = await readFile(new URL('./index.vue', import.meta.url), 'utf8')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1]
assert.ok(script, 'home should expose executable page state')
assert.match(source, /class="brand-logo"[^>]+src="\/static\/brand\/logo\.png"/, 'home should use the matching enneagram brand logo')
assert.match(source, /padding-top:var\(--status-bar-height, env\(safe-area-inset-top\)\)/, 'home custom navigation should clear the WeChat status bar')
const executable = script.replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*;?\s*$/gm, '')
const dir = await mkdtemp(join(tmpdir(), 'nx-studio-home-'))
const modulePath = join(dir, 'home-state.mjs')
await writeFile(modulePath, `
const ref = value => ({ value })
const computed = getter => ({ get value() { return getter() } })
const onMounted = handler => { globalThis.__homeHarness.mount = handler }
const getStoredSiteConfig = () => globalThis.__homeHarness.cache
const refreshSiteConfig = () => globalThis.__homeHarness.refresh()
const listClassroomRecentApi = query => globalThis.__homeHarness.list(query)
const normalizeTeachers = config => config.teachers || (config.home?.teacherTeaser ? [config.home.teacherTeaser] : [])
const normalizeCoursewareItems = config => config.home?.courses?.items || []
const normalizeMiniappLearn = config => ({ classroom: { enabled: config?.home?.miniappLearn?.classroom?.enabled !== false } })
const classroomContentRoute = item => /^[1-9]\\d*$/.test(String(item?.id || '')) ? '/pages/classroom-detail/classroom-detail?id=' + item.id + '&type=' + (item.contentType === 'audio' ? 'audio' : 'video') : ''
const setBookingIntent = value => { globalThis.__homeHarness.intent = value }
const clearBookingIntent = () => { globalThis.__homeHarness.intent = null }
const STUDIO_TEACHER = { name: '韩老师', avatar: '/static/teacher/portrait.jpg' }
const STUDIO_COURSES = [{ id: 'preview-course', title: '演示课程' }]
const UI_PREVIEW = globalThis.__homeHarness.preview
${executable}
export { config, videos, loading, error, teacher, portrait, isLaohan, courses, classroomEnabled, dailyVideos, load, daily, booking, openVideo, openCourse }
`)

let counter = 0
async function harness({ preview = false, cache = null } = {}) {
  const state = {
    preview, cache, intent: null, navigations: [], tabs: [],
    refresh: async () => state.cache || {},
    list: async () => ({ items: [] }),
  }
  globalThis.__homeHarness = state
  globalThis.uni = {
    navigateTo: options => state.navigations.push(options),
    switchTab: options => state.tabs.push(options),
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
    const cached = { teachers: [{ name: '已缓存老师' }], home: { courses: { items: [{ title: '正式课程' }] }, miniappLearn: { classroom: { enabled: false } } } }
    const { page } = await harness({ cache: cached })
    assert.equal(page.teacher.value.name, '已缓存老师', 'cached teacher should render before refresh')
    assert.equal(page.classroomEnabled.value, false, 'cached classroom.enabled=false should hide published video entries')
    assert.equal(page.courses.value[0].title, '正式课程', 'production should render configured courses rather than demo schedules')
    assert.deepEqual(page.videos.value, [], 'production should not invent a published video list')
  }
  {
    const { page } = await harness({ preview: true })
    assert.equal(page.courses.value[0].id, 'preview-course', 'explicit preview may display the isolated sample schedule')
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
  }
  console.log('teacher studio home state tests passed')
} finally {
  delete globalThis.__homeHarness
  delete globalThis.uni
  await rm(dir, { recursive: true, force: true })
}
