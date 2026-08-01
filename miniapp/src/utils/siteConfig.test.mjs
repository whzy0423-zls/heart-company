import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-site-config-'))
const modulePath = join(dir, 'siteConfig.mjs')
let source = await readFile(new URL('./siteConfig.js', import.meta.url), 'utf8')
source = source.replace("import { getSiteConfigApi } from '../api'", "const getSiteConfigApi = async () => ({ home: {} })")
await writeFile(modulePath, source)

let storage = {}
globalThis.uni = {
  getStorageSync(key) { return storage[key] || '' },
  setStorageSync(key, value) { storage[key] = value },
  removeStorageSync(key) { delete storage[key] },
}
let now = 100000
let calls = 0
const api = async () => {
  calls += 1
  return { home: { courses: { items: [`course-${calls}`] }, quotes: { items: [`quote-${calls}`] } } }
}

const {
  clearSiteConfigCache,
  getCachedSiteConfig,
  getStoredSiteConfig,
  hasSiteConfigLearningContent,
  hasSiteConfigLearningSection,
  refreshSiteConfig,
} = await import(`file://${modulePath}`)
assert.equal(getStoredSiteConfig(), null, 'empty storage should not expose cached config')
const first = await getCachedSiteConfig({ api, now: () => now, ttlMs: 60000 })
const second = await getCachedSiteConfig({ api, now: () => now + 1000, ttlMs: 60000 })
assert.equal(calls, 1, 'fresh cache should avoid a second site config request')
assert.deepEqual(second, first)
assert.deepEqual(getStoredSiteConfig(), first, 'stored config should be readable without awaiting network')

const refreshed = await refreshSiteConfig({ api, now: () => now + 2000 })
assert.equal(calls, 2, 'explicit refresh should bypass fresh cache')
assert.notDeepEqual(refreshed, first)
assert.deepEqual(getStoredSiteConfig(), refreshed, 'explicit refresh should update stored cache')
assert.equal(hasSiteConfigLearningContent(refreshed), true, 'site config with courses/quotes should count as learning content')
assert.equal(hasSiteConfigLearningContent({ home: {} }), false, 'empty site config should not replace cached learning content')
assert.equal(hasSiteConfigLearningContent({ teacher: { name: '韩老师' } }), true, 'root teacher profile should count as learning content')
assert.equal(hasSiteConfigLearningContent({ home: { teachers: [{ name: '韩老师' }] } }), true, 'home teachers should count as learning content')
assert.equal(hasSiteConfigLearningContent({ home: { teacherTeaser: { title: '韩常青（老韩）｜九型芯之力首席导师' } } }), true, 'home teacherTeaser should count as learning content')
assert.equal(hasSiteConfigLearningContent({ materials: [{ title: '课件' }] }), true, 'root materials should count as learning content')
assert.equal(hasSiteConfigLearningContent({ home: { courseware: { items: [{ title: '课件' }] } } }), true, 'home courseware should count as learning content')
assert.equal(hasSiteConfigLearningContent({ home: { courses: { items: [] }, quotes: { items: [] } } }), false, 'empty learning arrays should not count as visible learning content')
assert.equal(hasSiteConfigLearningSection({ home: {} }), false, 'missing learning section should be treated as incomplete')
assert.equal(hasSiteConfigLearningSection({ home: { teacher: {} } }), true, 'explicit teacher section should be treated as intentional content')
assert.equal(hasSiteConfigLearningSection({ home: { teacherTeaser: {} } }), true, 'explicit teacherTeaser should be treated as an intentional learning section')
assert.equal(hasSiteConfigLearningSection({ courses: { list: [] } }), true, 'courses.list should be treated as an intentional learning section')
assert.equal(hasSiteConfigLearningSection({ home: { courses: { items: [] }, quotes: { items: [] } } }), true, 'explicit empty learning arrays should be treated as intentional content')

const storedBeforeEmptyRefresh = getStoredSiteConfig()
const emptyRefreshResult = await refreshSiteConfig({
  api: async () => ({ home: { courses: { items: [] }, quotes: { items: [] } } }),
  now: () => now + 2500,
})
assert.equal(hasSiteConfigLearningContent(emptyRefreshResult), false, 'empty refresh payload should still be returned to the caller')
assert.deepEqual(
  getStoredSiteConfig(),
  emptyRefreshResult,
  'explicit empty learning arrays should overwrite cached learning content so admin clearing takes effect',
)

const storedBeforeMissingLearningRefresh = getStoredSiteConfig()
await refreshSiteConfig({
  api: async () => ({ home: {} }),
  now: () => now + 2600,
})
assert.deepEqual(
  getStoredSiteConfig(),
  storedBeforeMissingLearningRefresh,
  'missing learning sections should not overwrite the last complete learning cache',
)

now += 63000
const third = await getCachedSiteConfig({ api, now: () => now, ttlMs: 60000 })
assert.equal(calls, 3, 'expired cache should refetch')
assert.notDeepEqual(third, first)

clearSiteConfigCache()
await getCachedSiteConfig({ api, now: () => now, ttlMs: 60000 })
assert.equal(calls, 4, 'manual cache clear should refetch')

clearSiteConfigCache()
const teacherTeaserOnly = {
  home: {
    teacherTeaser: {
      eyebrow: '老师简介',
      title: '韩常青（老韩）｜九型芯之力首席导师',
      lead: '北京九型成长平台、芯之力创始人。',
      image: '/assets/teacher-poster.jpg',
    },
  },
}
await getCachedSiteConfig({ api: async () => teacherTeaserOnly, now: () => now + 1, ttlMs: 60000 })
await refreshSiteConfig({ api: async () => ({ home: {} }), now: () => now + 2 })
assert.deepEqual(
  getStoredSiteConfig(),
  teacherTeaserOnly,
  'a teacherTeaser-only cache should survive a later response that omits learning sections',
)

clearSiteConfigCache()
const completeLearningConfig = {
  home: {
    teacherTeaser: { title: '缓存老师', lead: '缓存老师介绍' },
    courses: { items: [{ title: '缓存课程' }] },
    quotes: { items: ['缓存语录'] },
  },
  theme: { issue: 'spring' },
}
await refreshSiteConfig({ api: async () => completeLearningConfig, now: () => now + 10 })

const teacherOnlyRefresh = { home: { teacherTeaser: { title: '更新老师' } } }
const teacherOnlySnapshot = structuredClone(teacherOnlyRefresh)
await refreshSiteConfig({ api: async () => teacherOnlyRefresh, now: () => now + 11 })
assert.deepEqual(teacherOnlyRefresh, teacherOnlySnapshot, 'section-aware cache merge must not mutate a teacher-only response')
assert.deepEqual(
  getStoredSiteConfig(),
  {
    home: {
      teacherTeaser: { title: '更新老师' },
      courses: { items: [{ title: '缓存课程' }] },
      quotes: { items: ['缓存语录'] },
    },
  },
  'a teacher-only refresh should preserve cached course and quote sections for the next mount',
)

const courseOnlyRefresh = { home: { courses: { items: [{ title: '更新课程' }] } } }
const courseOnlySnapshot = structuredClone(courseOnlyRefresh)
await refreshSiteConfig({ api: async () => courseOnlyRefresh, now: () => now + 12 })
assert.deepEqual(courseOnlyRefresh, courseOnlySnapshot, 'section-aware cache merge must not mutate a course-only response')
assert.deepEqual(
  getStoredSiteConfig(),
  {
    home: {
      teacherTeaser: { title: '更新老师' },
      courses: { items: [{ title: '更新课程' }] },
      quotes: { items: ['缓存语录'] },
    },
  },
  'a course-only refresh should preserve cached teacher and quote sections for the next mount',
)

const explicitEmptyRefresh = {
  home: {
    teacherTeaser: {},
    courses: { items: [] },
    quotes: { items: [] },
  },
}
const explicitEmptySnapshot = structuredClone(explicitEmptyRefresh)
await refreshSiteConfig({ api: async () => explicitEmptyRefresh, now: () => now + 13 })
assert.deepEqual(explicitEmptyRefresh, explicitEmptySnapshot, 'explicit empty section caching must not mutate the response')
assert.deepEqual(
  getStoredSiteConfig(),
  explicitEmptyRefresh,
  'explicit empty teacher, course, and quote sections should clear their cached counterparts',
)

console.log('site config cache tests passed')
await rm(dir, { force: true, recursive: true })
