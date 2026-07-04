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

console.log('booking draft tests passed')
await rm(dir, { force: true, recursive: true })
