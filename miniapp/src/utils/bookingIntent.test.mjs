import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-booking-intent-'))
const modulePath = join(dir, 'bookingIntent.mjs')
const source = await readFile(new URL('./bookingIntent.js', import.meta.url), 'utf8')
await writeFile(modulePath, source)

let storage = {}
let throwGet = false
let throwSet = false
let throwRemove = false
const removedKeys = []
globalThis.uni = {
  getStorageSync(key) {
    if (throwGet) throw new Error('read unavailable')
    return storage[key]
  },
  setStorageSync(key, value) {
    if (throwSet) throw new Error('write unavailable')
    storage[key] = value
  },
  removeStorageSync(key) {
    removedKeys.push(key)
    if (throwRemove) throw new Error('remove unavailable')
    delete storage[key]
  },
}

const {
  BOOKING_INTENT_KEY,
  setBookingIntent,
  consumeBookingIntent,
  clearBookingIntent,
} = await import(`file://${modulePath}`)

assert.notEqual(BOOKING_INTENT_KEY, 'nx_booking_draft', 'intent must use an independent storage key')
assert.equal(setBookingIntent({ kind: 'consult', intentText: '  想了解咨询  ', url: '/pages/ignored' }), true)
assert.deepEqual(storage[BOOKING_INTENT_KEY], {
  kind: 'consult',
  intentText: '想了解咨询',
}, 'only normalized intent fields are persisted')
assert.deepEqual(consumeBookingIntent(), { kind: 'consult', intentText: '想了解咨询' }, 'a saved intent is consumed once')
assert.equal(storage[BOOKING_INTENT_KEY], undefined, 'consumption removes the stored value')
assert.equal(consumeBookingIntent(), null, 'a second consumption has no value')

for (const kind of ['consult', 'course', 'enterprise']) {
  assert.equal(setBookingIntent({ kind, intentText: kind }), true, `${kind} is supported`)
  assert.deepEqual(consumeBookingIntent(), { kind, intentText: kind }, `${kind} survives normalization`)
}

storage[BOOKING_INTENT_KEY] = { kind: 'consult', intentText: '旧意图', timestamp: 1 }
assert.equal(setBookingIntent({ kind: 'unknown', intentText: 'new' }), false, 'unknown kinds are rejected')
assert.equal(storage[BOOKING_INTENT_KEY], undefined, 'invalid kinds clear old intents')
storage[BOOKING_INTENT_KEY] = { kind: 'consult', intentText: '旧意图', timestamp: 1 }
assert.equal(setBookingIntent({ kind: '   ', intentText: 'new' }), false, 'empty kinds are rejected')
assert.equal(storage[BOOKING_INTENT_KEY], undefined, 'empty kinds clear old intents')

const longText = `  ${'意'.repeat(121)}  `
assert.equal(setBookingIntent({ kind: 'enterprise', intentText: longText }), true)
assert.deepEqual(consumeBookingIntent(), { kind: 'enterprise', intentText: '意'.repeat(120) }, 'intent text is trimmed and capped at 120 chars')

const emojiText = `${'😀'.repeat(120)}😀`
assert.equal(setBookingIntent({ kind: 'enterprise', intentText: emojiText }), true)
const emojiIntent = consumeBookingIntent()
assert.equal(Array.from(emojiIntent.intentText).length, 120, 'emoji text is capped by Unicode code points')
assert.equal(emojiIntent.intentText, '😀'.repeat(120), 'emoji truncation does not leave a lone surrogate')
assert.equal(setBookingIntent({ kind: 'course', intentText: 42 }), true)
assert.deepEqual(consumeBookingIntent(), { kind: 'course', intentText: '' }, 'non-string intent text normalizes to empty text')

storage[BOOKING_INTENT_KEY] = JSON.stringify({ kind: 'enterprise', intentText: ' JSON ', timestamp: 1, extra: 'ignore' })
assert.deepEqual(consumeBookingIntent(), { kind: 'enterprise', intentText: 'JSON' }, 'serialized storage is parsed and normalized')
const original = { kind: ' course ', intentText: ' 原对象 ', nested: { untouched: true } }
assert.equal(setBookingIntent(original), true)
assert.deepEqual(original, { kind: ' course ', intentText: ' 原对象 ', nested: { untouched: true } }, 'setting does not mutate its input')
clearBookingIntent()
assert.equal(storage[BOOKING_INTENT_KEY], undefined, 'clear removes a stored intent')

storage[BOOKING_INTENT_KEY] = { kind: 'consult', intentText: 'stale after failed write' }
throwSet = true
let removeCount = removedKeys.length
assert.equal(setBookingIntent({ kind: 'consult', intentText: 'storage fails' }), false, 'write failures return false')
assert.equal(removedKeys.length, removeCount + 1, 'write failures attempt exactly one cleanup')
assert.equal(storage[BOOKING_INTENT_KEY], undefined, 'write failures clear stale intent')
throwSet = false

storage[BOOKING_INTENT_KEY] = { kind: 'consult', intentText: 'read fails' }
throwGet = true
removeCount = removedKeys.length
assert.equal(consumeBookingIntent(), null, 'read failures return null')
assert.equal(removedKeys.length, removeCount + 1, 'read failures attempt exactly one cleanup')
throwGet = false
assert.equal(storage[BOOKING_INTENT_KEY], undefined, 'read failure cleanup removes the stored intent')

storage[BOOKING_INTENT_KEY] = '{broken json'
removeCount = removedKeys.length
assert.equal(consumeBookingIntent(), null, 'corrupt JSON returns no intent')
assert.equal(removedKeys.length, removeCount + 1, 'corrupt JSON attempts exactly one cleanup')
assert.equal(storage[BOOKING_INTENT_KEY], undefined, 'corrupt JSON is still cleared')

storage[BOOKING_INTENT_KEY] = { kind: 'consult', intentText: 'remove fails' }
throwRemove = true
removeCount = removedKeys.length
assert.equal(consumeBookingIntent(), null, 'remove failures degrade consumption to null')
assert.equal(removedKeys.length, removeCount + 1, 'remove failures are attempted exactly once')
assert.doesNotThrow(() => clearBookingIntent(), 'clear swallows removal failures')
throwRemove = false

console.log('booking intent tests passed')
await rm(dir, { force: true, recursive: true })
