import assert from 'node:assert/strict'
import { copyFile, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-booking-display-'))
try {
  const modulePath = join(dir, 'bookingDisplay.mjs')
  await copyFile(new URL('./bookingDisplay.js', import.meta.url), modulePath)
  const {
    bookingKindLabel,
    bookingStatusLabel,
    bookingValue,
    maskBookingPhone,
    normalizeBookingId,
  } = await import(`file://${modulePath}`)

  assert.equal(normalizeBookingId(42), '42')
  assert.equal(normalizeBookingId('42'), normalizeBookingId(42))
  assert.equal(normalizeBookingId('  %34%32  '), '42')
  assert.equal(normalizeBookingId(''), '')
  assert.equal(normalizeBookingId('   '), '')
  assert.equal(normalizeBookingId('abc'), '')
  assert.equal(normalizeBookingId('1.5'), '')
  assert.equal(normalizeBookingId('-1'), '')
  assert.equal(normalizeBookingId('+1'), '')
  assert.equal(normalizeBookingId('%'), '')
  assert.equal(normalizeBookingId('%E0%A4%A'), '')
  assert.equal(normalizeBookingId(null), '')

  assert.equal(bookingKindLabel('consult'), '1v1 咨询')
  assert.equal(bookingKindLabel('course'), '课程报名')
  assert.equal(bookingKindLabel('enterprise'), '企业课程')
  assert.equal(bookingKindLabel('future-kind'), 'future-kind')
  assert.equal(bookingKindLabel('constructor'), 'constructor')
  assert.equal(bookingKindLabel('toString'), 'toString')
  assert.equal(bookingKindLabel('__proto__'), '__proto__')
  assert.equal(bookingKindLabel('  '), '未填写')

  assert.equal(bookingStatusLabel('pending'), '待确认')
  assert.equal(bookingStatusLabel('confirmed'), '已确认')
  assert.equal(bookingStatusLabel('completed'), '已完成')
  assert.equal(bookingStatusLabel('cancelled'), '已取消')
  assert.equal(bookingStatusLabel('future-status'), 'future-status')
  assert.equal(bookingStatusLabel('constructor'), 'constructor')
  assert.equal(bookingStatusLabel('toString'), 'toString')
  assert.equal(bookingStatusLabel('__proto__'), '__proto__')
  assert.equal(bookingStatusLabel(null), '未填写')

  const nativeHasOwn = Object.hasOwn
  try {
    Object.hasOwn = undefined
    assert.equal(bookingKindLabel('consult'), '1v1 咨询', 'kind labels should work without Object.hasOwn support')
    assert.equal(bookingStatusLabel('pending'), '待确认', 'status labels should work without Object.hasOwn support')
  } finally {
    Object.hasOwn = nativeHasOwn
  }

  assert.equal(bookingValue('学习沟通'), '学习沟通')
  assert.equal(bookingValue(0), '0')
  assert.equal(bookingValue(false), 'false')
  assert.equal(bookingValue('   '), '未填写')
  assert.equal(bookingValue(undefined), '未填写')

  assert.equal(maskBookingPhone('13800138000'), '138****8000')
  assert.equal(maskBookingPhone(13800138000), '138****8000')
  assert.equal(maskBookingPhone('123456'), '123456')
  assert.equal(maskBookingPhone('138-0013-8000'), '138-0013-8000')
  assert.equal(maskBookingPhone('phone-number'), 'phone-number')
  assert.equal(maskBookingPhone('138****8000'), '138****8000')
  assert.equal(maskBookingPhone('  '), '未填写')
  assert.equal(maskBookingPhone(null), '未填写')

  console.log('booking display tests passed')
} finally {
  await rm(dir, { force: true, recursive: true })
}
