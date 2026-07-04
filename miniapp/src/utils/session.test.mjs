import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-session-'))
const modulePath = join(dir, 'session.mjs')
const source = await readFile(new URL('./session.js', import.meta.url), 'utf8')
await writeFile(modulePath, source)

let storage = {}
globalThis.uni = {
  getStorageSync(key) { return storage[key] || '' },
  setStorageSync(key, value) { storage[key] = value },
  removeStorageSync(key) { delete storage[key] },
}

const mod = await import(`file://${modulePath}`)
const result = { type: 5, second: 6, score: { 5: 12 }, centers: [{ key: 'head', pct: 80 }] }
mod.setLastResult(result, 'male')
assert.deepEqual(mod.getLastResult(), { result, gender: 'male' })

const freshModulePath = join(dir, 'session-fresh.mjs')
await writeFile(freshModulePath, source)
const fresh = await import(`file://${freshModulePath}`)
assert.deepEqual(fresh.getLastResult(), { result, gender: 'male' }, 'last test result should survive module reload via storage')

fresh.clearLastResult()
assert.deepEqual(fresh.getLastResult(), { result: null, gender: null })

console.log('session tests passed')
await rm(dir, { force: true, recursive: true })
