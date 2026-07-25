import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-booking-session-'))

try {
  const displaySource = await readFile(new URL('./bookingDisplay.js', import.meta.url), 'utf8')
  await writeFile(join(dir, 'bookingDisplay.mjs'), displaySource)

  const sessionSource = await readFile(new URL('./bookingSession.js', import.meta.url), 'utf8')
  await writeFile(
    join(dir, 'bookingSession.mjs'),
    sessionSource.replace("'./bookingDisplay.js'", "'./bookingDisplay.mjs'"),
  )

  const { clearBookingSession, readBookingSession, setBookingSession } = await import(
    `file://${join(dir, 'bookingSession.mjs')}`
  )

  const booking = {
    id: 42,
    kind: 'consult',
    contactName: '小九',
  }

  assert.equal(setBookingSession('token-a', booking), true)
  assert.deepEqual(readBookingSession('token-a', '42'), booking)

  clearBookingSession()
  assert.equal(setBookingSession('token-a', { ...booking, id: '42' }), true)
  assert.equal(readBookingSession('token-a', 42)?.id, '42')

  clearBookingSession()
  setBookingSession('token-a', booking)
  assert.equal(readBookingSession('token-b', 42), null)
  assert.equal(readBookingSession('token-a', 42), null)

  setBookingSession('token-a', booking)
  assert.equal(readBookingSession('token-a', 43), null)
  assert.equal(readBookingSession('token-a', 42), null)

  const invalidCases = [
    ['', booking],
    ['   ', booking],
    [null, booking],
    ['token-a', null],
    ['token-a', []],
    ['token-a', {}],
    ['token-a', { id: '' }],
    ['token-a', { id: 'abc' }],
    ['token-a', { id: '1.5' }],
  ]

  for (const [ownerToken, record] of invalidCases) {
    setBookingSession('token-a', booking)
    assert.equal(setBookingSession(ownerToken, record), false)
    assert.equal(readBookingSession('token-a', 42), null)
  }

  setBookingSession('token-a', booking)
  const returned = readBookingSession('token-a', 42)
  returned.contactName = '被修改'
  assert.equal(readBookingSession('token-a', 42)?.contactName, '小九')

  setBookingSession('token-a', booking)
  clearBookingSession()
  assert.equal(readBookingSession('token-a', 42), null)

  console.log('booking session tests passed')
} finally {
  await rm(dir, { force: true, recursive: true })
}
