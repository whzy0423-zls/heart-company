import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-learning-nav-intent-'))
const modulePath = join(dir, 'learningNavIntent.mjs')
const source = await readFile(new URL('./learningNavIntent.js', import.meta.url), 'utf8')
await writeFile(modulePath, source)

let storage = {}
let failNextWrite = false
let failRemovals = false
globalThis.uni = {
  getStorageSync(key) {
    return Object.prototype.hasOwnProperty.call(storage, key) ? storage[key] : ''
  },
  setStorageSync(key, value) {
    if (failNextWrite) {
      failNextWrite = false
      throw new Error('storage write failed')
    }
    storage[key] = value
  },
  removeStorageSync(key) {
    if (failRemovals) throw new Error('storage removal failed')
    delete storage[key]
  },
}

const {
  LEARNING_NAV_INTENT_KEY,
  LEARNING_NAV_INTENT_TTL_MS,
  clearLearningNavIntent,
  readLearningNavIntent,
  setLearningNavIntent,
} = await import(`file://${modulePath}`)

assert.equal(LEARNING_NAV_INTENT_TTL_MS, 10_000, 'learning navigation intent should expire after ten seconds')

for (const value of ['course', 'material', 'quote']) {
  setLearningNavIntent(value, { now: () => 1_000 })
  assert.deepEqual(storage[LEARNING_NAV_INTENT_KEY], {
    value,
    expiresAt: 11_000,
  }, `${value} should be stored with a ten-second expiry`)
  assert.equal(readLearningNavIntent({ now: () => 10_999 }), value, `${value} should be readable before expiry`)
  assert.equal(storage[LEARNING_NAV_INTENT_KEY], undefined, 'reading should clear the navigation intent')
}

storage[LEARNING_NAV_INTENT_KEY] = { value: 'material', expiresAt: 11_000 }
assert.equal(readLearningNavIntent({ now: () => 11_000 }), null, 'an expired intent should be ignored')
assert.equal(storage[LEARNING_NAV_INTENT_KEY], undefined, 'an expired intent should be cleared automatically')

setLearningNavIntent('course', { now: () => 1_000 })
failRemovals = true
assert.equal(readLearningNavIntent({ now: () => 1_000 }), 'course', 'a valid intent may still be returned when persistent cleanup fails')
assert.equal(readLearningNavIntent({ now: () => 1_000 }), null, 'the same intent should not be consumed twice when persistent cleanup fails')
failRemovals = false
delete storage[LEARNING_NAV_INTENT_KEY]

for (const value of ['', 'lesson', 'COURSE', null, undefined]) {
  storage[LEARNING_NAV_INTENT_KEY] = { value: 'course', expiresAt: 99_000 }
  assert.equal(setLearningNavIntent(value, { now: () => 1_000 }), false, 'invalid values should be rejected')
  assert.equal(storage[LEARNING_NAV_INTENT_KEY], undefined, 'rejecting an invalid value should clear stale intent')
}

storage[LEARNING_NAV_INTENT_KEY] = { value: 'course', expiresAt: 99_000 }
failNextWrite = true
assert.equal(setLearningNavIntent('material', { now: () => 1_000 }), false, 'a failed storage write should be reported')
assert.equal(storage[LEARNING_NAV_INTENT_KEY], undefined, 'a failed write should clear the older navigation intent')

for (const invalid of [
  'not-json',
  false,
  0,
  { value: 'lesson', expiresAt: 99_000 },
  { value: 'course', expiresAt: 'soon' },
  { value: 'course' },
]) {
  storage[LEARNING_NAV_INTENT_KEY] = invalid
  assert.equal(readLearningNavIntent({ now: () => 1_000 }), null, 'invalid stored data should fall back to no intent')
  assert.equal(storage[LEARNING_NAV_INTENT_KEY], undefined, 'invalid stored data should be cleared')
}

setLearningNavIntent('quote', { now: () => 1_000 })
clearLearningNavIntent()
assert.equal(storage[LEARNING_NAV_INTENT_KEY], undefined, 'explicit clear should remove the navigation intent')

console.log('learning navigation intent tests passed')
await rm(dir, { force: true, recursive: true })
