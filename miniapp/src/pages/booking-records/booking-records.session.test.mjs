import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const source = await readFile(new URL('./booking-records.vue', import.meta.url), 'utf8')

for (const token of [
  '--nx-brand-900',
  '--nx-brand-700',
  '--nx-accent-gold',
  '--nx-page-bg',
  '--nx-surface',
  '--nx-surface-soft',
  '--nx-text',
  '--nx-text-muted',
  '--nx-border',
]) {
  assert.ok(source.includes(`var(${token})`), `booking records page should use ${token}`)
}
assert.match(source, /<view class="wrap booking-records page-stack ios-page ios-safe-bottom">/, 'booking records should use the shared page shell')
assert.doesNotMatch(
  source,
  /#(?:60a5fa|7c3aed|2b7fff|6d5dfc|2563eb|295fbd|e8f1ff)/i,
  'booking records should not keep the old orange-blue or violet theme',
)
assert.match(source, /v-if="loading"/, 'booking records loading state should remain available')
assert.match(source, /v-else-if="loadError"/, 'booking records error state should remain available')
assert.match(source, /v-else-if="bookings\.length === 0"/, 'booking records empty state should remain available')
assert.match(source, /@click\.stop="retryLoad">重试/, 'booking records errors should keep a retry action')
assert.match(source, /\.retry-button,[\s\S]*?min-height:\s*88rpx/, 'booking records actions should keep full-size touch targets')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1]
assert.ok(script, 'booking records page should expose a script setup block')

const executableScript = script.replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*$/gm, '')
const dir = await mkdtemp(join(tmpdir(), 'nx-booking-records-session-'))
const modulePath = join(dir, 'booking-records-session.mjs')

const harnessPrelude = `
const ref = (value) => ({ value })
const onShow = (handler) => { globalThis.__bookingRecordsHarness.onShow = handler }
const onUnload = (handler) => { globalThis.__bookingRecordsHarness.onUnload = handler }
const listBookingsApi = () => globalThis.__bookingRecordsHarness.listBookingsApi()
const clearToken = () => globalThis.__bookingRecordsHarness.clearToken()
const getToken = () => globalThis.__bookingRecordsHarness.token
const bookingKindLabel = (value) => String(value || '')
const bookingStatusLabel = (value) => String(value || '')
const bookingValue = (value) => value == null || String(value).trim() === '' ? '未填写' : String(value).trim()
const maskBookingPhone = (value) => String(value || '')
const normalizeBookingId = (value) => /^\\d+$/.test(String(value || '')) ? String(value) : ''
const clearBookingSession = () => globalThis.__bookingRecordsHarness.clearBookingSession()
const setBookingSession = (token, record) => globalThis.__bookingRecordsHarness.setBookingSession(token, record)
const userErrorMessage = (error, fallback) => error?.message || fallback
`

await writeFile(
  modulePath,
  `${harnessPrelude}\n${executableScript}\nexport { bookings, loading, loadError, loadBookings, openBooking }\n`,
)

let moduleCounter = 0

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, reject, resolve }
}

async function createHarness() {
  const state = {
    token: '',
    clearTokenCalls: 0,
    sessionClearCalls: 0,
    sessionSets: [],
    toasts: [],
    switches: [],
    navigations: [],
    navigateFailure: false,
    listBookingsApi: async () => ({ items: [] }),
    clearToken() {
      this.clearTokenCalls += 1
      this.token = ''
    },
    clearBookingSession() {
      this.sessionClearCalls += 1
    },
    setBookingSession(token, record) {
      this.sessionSets.push({ record, token })
      return true
    },
  }

  globalThis.__bookingRecordsHarness = state
  globalThis.uni = {
    showToast(options) { state.toasts.push(options) },
    switchTab(options) { state.switches.push(options) },
    navigateTo(options) {
      state.navigations.push(options)
      if (state.navigateFailure) options.fail?.(new Error('navigation failed'))
    },
  }

  moduleCounter += 1
  const page = await import(`${pathToFileURL(modulePath).href}?case=${moduleCounter}`)
  return { page, state }
}

try {
  {
    const { page, state } = await createHarness()
    const pending = deferred()
    const oldRecord = { id: 1, kind: 'consult' }
    state.token = 'token-a'
    state.listBookingsApi = () => pending.promise

    const loadPromise = page.loadBookings()
    state.token = 'token-b'
    pending.resolve({ items: [oldRecord] })
    await loadPromise

    assert.equal(state.token, 'token-b', 'an old successful response must not clear a newer token')
    assert.deepEqual(page.bookings.value, [], 'an old successful response must not expose the old user booking list')
    assert.equal(state.sessionClearCalls, 1, 'an old successful response should invalidate the old booking session')
    assert.equal(state.toasts.length, 0, 'an old successful response must not show an auth-expired Toast')
    assert.equal(state.switches.length, 0, 'an old successful response must not redirect the newer session')
  }

  {
    const { page, state } = await createHarness()
    const pending = deferred()
    state.token = 'token-a'
    state.listBookingsApi = () => pending.promise

    const loadPromise = page.loadBookings()
    state.token = 'token-b'
    pending.reject(Object.assign(new Error('Unauthorized'), {
      authExpired: true,
      authSessionCurrent: false,
      requestToken: 'token-a',
      statusCode: 401,
    }))
    await loadPromise

    assert.equal(state.token, 'token-b', 'an old 401 must not clear a newer token')
    assert.deepEqual(page.bookings.value, [], 'an old 401 should invalidate the old user booking list')
    assert.equal(state.sessionClearCalls, 1, 'an old 401 should clear the old booking session')
    assert.equal(state.toasts.length, 0, 'an old 401 must not show an auth-expired Toast')
    assert.equal(state.switches.length, 0, 'an old 401 must not redirect the newer session')
  }

  {
    const { page, state } = await createHarness()
    const oldRecord = { id: 7, kind: 'course' }
    state.token = 'token-a'
    state.listBookingsApi = async () => ({ items: [oldRecord] })
    await page.loadBookings()
    state.sessionClearCalls = 0

    state.token = 'token-b'
    page.openBooking(oldRecord)

    assert.equal(state.token, 'token-b', 'clicking an old-token record must not clear the newer token')
    assert.deepEqual(page.bookings.value, [], 'clicking an old-token record should clear the old list')
    assert.equal(state.sessionClearCalls, 1, 'clicking an old-token record should clear the old booking session')
    assert.equal(state.sessionSets.length, 0, 'clicking an old-token record must not bind it to the newer token')
    assert.equal(state.navigations.length, 0, 'clicking an old-token record must not navigate to its detail')
    assert.equal(state.toasts.length, 0, 'clicking an old-token record must not show an auth-expired Toast')
    assert.equal(state.switches.length, 0, 'clicking an old-token record must not redirect the newer session')
  }

  {
    const { page, state } = await createHarness()
    const pending = deferred()
    state.token = 'token-a'
    state.listBookingsApi = () => pending.promise

    const loadPromise = page.loadBookings()
    state.token = ''
    pending.reject(Object.assign(new Error('Unauthorized'), {
      authExpired: true,
      authSessionCurrent: true,
      requestToken: 'token-a',
      statusCode: 401,
    }))
    await loadPromise

    assert.equal(state.clearTokenCalls, 1, 'a current-session 401 should keep auth cleanup centralized')
    assert.equal(state.sessionClearCalls, 1, 'a current-session 401 should clear booking session data')
    assert.equal(state.toasts.length, 1, 'a current-session 401 should show one auth-expired Toast')
    assert.equal(state.switches.length, 1, 'a current-session 401 should redirect once')
  }

  {
    const { page, state } = await createHarness()
    const record = { id: 8, kind: 'consult' }
    state.token = 'token-a'
    state.listBookingsApi = async () => ({ items: [record] })
    await page.loadBookings()
    state.sessionClearCalls = 0
    state.navigateFailure = true

    page.openBooking(record)

    assert.equal(state.sessionSets.length, 1, 'opening a record should bind it before navigation')
    assert.equal(state.navigations.length, 1, 'opening a record should attempt detail navigation once')
    assert.equal(state.sessionClearCalls, 1, 'failed detail navigation should clear the bound booking session')
  }

  console.log('booking records session tests passed')
} finally {
  delete globalThis.__bookingRecordsHarness
  delete globalThis.uni
  await rm(dir, { force: true, recursive: true })
}
