import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-booking-draft-'))
const modulePath = join(dir, 'bookingDraft.mjs')
const source = await readFile(new URL('./bookingDraft.js', import.meta.url), 'utf8')
await writeFile(modulePath, source)

let storage = {}
globalThis.uni = {
  getStorageSync(key) { return storage[key] || '' },
  setStorageSync(key, value) { storage[key] = value },
  removeStorageSync(key) { delete storage[key] },
}

const { loadBookingDraft, saveBookingDraft, clearBookingDraft, BOOKING_DRAFT_KEY } = await import(`file://${modulePath}`)

assert.deepEqual(loadBookingDraft(), null, 'empty storage should not return a draft')

storage[BOOKING_DRAFT_KEY] = { ts: 1, data: { kind: 'consult' } }
assert.equal(loadBookingDraft(), null, 'the untouched default form should not be restored as a draft')

storage[BOOKING_DRAFT_KEY] = { ts: 1, data: { kind: '   ' } }
assert.equal(loadBookingDraft(), null, 'a whitespace-only booking kind should normalize to the default kind')

storage[BOOKING_DRAFT_KEY] = { ts: 1, data: { kind: ' consult ' } }
assert.equal(loadBookingDraft(), null, 'a padded default kind should not make an untouched form meaningful')

storage[BOOKING_DRAFT_KEY] = {
  ts: 1,
  data: { kind: 'consult', contactName: '  ', phone: '\n', intent: '', preferredTime: '', message: '\t' },
}
assert.equal(loadBookingDraft(), null, 'whitespace-only default fields should not count as a meaningful draft')

storage[BOOKING_DRAFT_KEY] = { ts: 1, data: { kind: 'course' } }
assert.deepEqual(loadBookingDraft(), {
  kind: 'course',
  contactName: '',
  phone: '',
  intent: '',
  preferredTime: '',
  message: '',
}, 'a non-default booking kind should remain a meaningful draft')

storage[BOOKING_DRAFT_KEY] = { ts: 1, data: { kind: 'enterprise' } }
assert.equal(loadBookingDraft(), null, 'an untouched enterprise default form should not be restored as a draft')

storage[BOOKING_DRAFT_KEY] = { ts: 1, data: { kind: 'enterprise', intent: '企业内训' } }
assert.deepEqual(loadBookingDraft(), {
  kind: 'enterprise',
  contactName: '',
  phone: '',
  intent: '企业内训',
  preferredTime: '',
  message: '',
}, 'enterprise service preselection should remain a meaningful draft when it carries an intent')

storage[BOOKING_DRAFT_KEY] = { ts: 1, data: { kind: 'consult', message: ' 需要回电 ' } }
assert.equal(loadBookingDraft().message, ' 需要回电 ', 'any non-blank field should keep the default-kind draft meaningful')

saveBookingDraft({
  kind: 'course',
  contactName: ' 小九 ',
  phone: '13800138000',
  intent: '亲子关系',
  preferredTime: '周末',
  message: '想先了解课程',
  ignored: 'not persisted',
})

assert.equal(typeof storage[BOOKING_DRAFT_KEY].ts, 'number', 'draft should include a write timestamp')
assert.deepEqual(storage[BOOKING_DRAFT_KEY].data, {
  kind: 'course',
  contactName: ' 小九 ',
  phone: '13800138000',
  intent: '亲子关系',
  preferredTime: '周末',
  message: '想先了解课程',
})
assert.deepEqual(loadBookingDraft(), storage[BOOKING_DRAFT_KEY].data, 'saved draft should load back')

storage[BOOKING_DRAFT_KEY] = JSON.stringify({ ts: 1, data: { kind: 'enterprise', contactName: '企业', phone: '13900139000' } })
assert.deepEqual(loadBookingDraft(), {
  kind: 'enterprise',
  contactName: '企业',
  phone: '13900139000',
  intent: '',
  preferredTime: '',
  message: '',
}, 'string storage should be parsed and normalized')

clearBookingDraft()
assert.equal(storage[BOOKING_DRAFT_KEY], undefined, 'clear should remove persisted draft')

storage[BOOKING_DRAFT_KEY] = { ts: 1, data: { kind: 'consult', contactName: '旧草稿' } }
saveBookingDraft({ kind: 'consult', contactName: '  ', phone: '', intent: '', preferredTime: '', message: '' })
assert.equal(storage[BOOKING_DRAFT_KEY], undefined, 'saving an empty default form should clear the prior draft')

storage[BOOKING_DRAFT_KEY] = '{broken json'
assert.equal(loadBookingDraft(), null, 'corrupt serialized storage should be ignored')

storage[BOOKING_DRAFT_KEY] = { ts: 1, data: { kind: 'future-kind', contactName: '未来类型' } }
assert.equal(loadBookingDraft().kind, 'future-kind', 'unknown kinds should remain data-safe for the page fallback guard')

storage[BOOKING_DRAFT_KEY] = { ts: 1, data: { kind: ' future-kind ' } }
assert.equal(loadBookingDraft().kind, 'future-kind', 'meaningful unknown kinds should remain forward-compatible after trimming')

globalThis.uni = {
  getStorageSync() { throw new Error('storage unavailable') },
  setStorageSync() { throw new Error('storage unavailable') },
  removeStorageSync() { throw new Error('storage unavailable') },
}
assert.equal(loadBookingDraft(), null, 'storage read failures should degrade to no draft')
assert.doesNotThrow(
  () => saveBookingDraft({ kind: 'course', contactName: '测试异常', phone: '', intent: '', preferredTime: '', message: '' }),
  'storage write failures should not interrupt form editing',
)
assert.doesNotThrow(() => clearBookingDraft(), 'storage removal failures should not escape')

console.log('booking draft tests passed')
await rm(dir, { force: true, recursive: true })
