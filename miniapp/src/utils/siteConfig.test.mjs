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

const { getCachedSiteConfig, clearSiteConfigCache } = await import(`file://${modulePath}`)
const first = await getCachedSiteConfig({ api, now: () => now, ttlMs: 60000 })
const second = await getCachedSiteConfig({ api, now: () => now + 1000, ttlMs: 60000 })
assert.equal(calls, 1, 'fresh cache should avoid a second site config request')
assert.deepEqual(second, first)

now += 61000
const third = await getCachedSiteConfig({ api, now: () => now, ttlMs: 60000 })
assert.equal(calls, 2, 'expired cache should refetch')
assert.notDeepEqual(third, first)

clearSiteConfigCache()
await getCachedSiteConfig({ api, now: () => now, ttlMs: 60000 })
assert.equal(calls, 3, 'manual cache clear should refetch')

console.log('site config cache tests passed')
await rm(dir, { force: true, recursive: true })
