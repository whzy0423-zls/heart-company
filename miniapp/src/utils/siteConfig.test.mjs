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
assert.equal(
  hasSiteConfigLearningContent({ home: { teacherTeaser: { title: '韩老师', lead: '课程导学' } } }),
  true,
  'legacy home teacher teaser should count as learning content',
)
assert.equal(hasSiteConfigLearningContent({ materials: [{ title: '课件' }] }), true, 'root materials should count as learning content')
assert.equal(hasSiteConfigLearningContent({ home: { courseware: { items: [{ title: '课件' }] } } }), true, 'home courseware should count as learning content')
assert.equal(hasSiteConfigLearningContent({ home: { courses: { items: [] }, quotes: { items: [] } } }), false, 'empty learning arrays should not count as visible learning content')
assert.equal(hasSiteConfigLearningSection({ home: {} }), false, 'missing learning section should be treated as incomplete')
assert.equal(hasSiteConfigLearningSection({ home: { teacher: {} } }), true, 'explicit teacher section should be treated as intentional content')
assert.equal(
  hasSiteConfigLearningSection({ home: { teacherTeaser: {} } }),
  true,
  'explicit legacy teacher teaser should be treated as intentional content',
)
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

console.log('site config cache tests passed')
await rm(dir, { force: true, recursive: true })
