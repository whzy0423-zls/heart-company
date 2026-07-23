import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, stat, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const pageUrl = new URL('./booking-detail.vue', import.meta.url)
assert.ok((await stat(pageUrl, { throwIfNoEntry: false }))?.isFile(), 'booking detail page should exist before its session behavior can run')

const source = await readFile(pageUrl, 'utf8')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1]
assert.ok(script, 'booking detail page should expose a script setup block')

const executableScript = script.replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*$/gm, '')
const dir = await mkdtemp(join(tmpdir(), 'nx-booking-detail-session-'))
const modulePath = join(dir, 'booking-detail-session.mjs')

const harnessPrelude = `
const ref = (value) => ({ value })
const onLoad = (handler) => { globalThis.__bookingDetailHarness.onLoad = handler }
const onShow = (handler) => { globalThis.__bookingDetailHarness.onShow = handler }
const onHide = (handler) => { globalThis.__bookingDetailHarness.onHide = handler }
const onUnload = (handler) => { globalThis.__bookingDetailHarness.onUnload = handler }
const listBookingsApi = () => globalThis.__bookingDetailHarness.listBookingsApi()
const clearToken = () => globalThis.__bookingDetailHarness.clearToken()
const getToken = () => globalThis.__bookingDetailHarness.token
const bookingKindLabel = (value) => String(value || '')
const bookingStatusLabel = (value) => String(value || '')
const bookingValue = (value) => value == null || String(value).trim() === '' ? '未填写' : String(value).trim()
const normalizeBookingId = (value) => {
  if (value === null || value === undefined) return ''
  try {
    const normalized = decodeURIComponent(String(value).trim()).trim()
    return /^\\d+$/.test(normalized) ? normalized : ''
  } catch {
    return ''
  }
}
const clearBookingSession = () => globalThis.__bookingDetailHarness.clearBookingSession()
const readBookingSession = (token, bookingId) => globalThis.__bookingDetailHarness.readBookingSession(token, bookingId)
const userErrorMessage = (error, fallback) => error?.message || fallback
`

await writeFile(
  modulePath,
  `${harnessPrelude}\n${executableScript}\nexport { booking, loading, loadError, notFound, loadBookingDetail, retryLoad }\n`,
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
    sessionReads: [],
    cachedRecord: null,
    listCalls: 0,
    toasts: [],
    switches: [],
    redirects: [],
    listBookingsApi: async () => ({ items: [] }),
    clearToken() {
      this.clearTokenCalls += 1
      this.token = ''
    },
    clearBookingSession() {
      this.sessionClearCalls += 1
      this.cachedRecord = null
    },
    readBookingSession(token, bookingId) {
      this.sessionReads.push({ bookingId, token })
      return this.cachedRecord ? { ...this.cachedRecord } : null
    },
  }

  const listImplementation = state.listBookingsApi
  state.listBookingsApi = async () => {
    state.listCalls += 1
    return listImplementation()
  }

  globalThis.__bookingDetailHarness = state
  globalThis.uni = {
    showToast(options) { state.toasts.push(options) },
    switchTab(options) { state.switches.push(options) },
    redirectTo(options) { state.redirects.push(options) },
  }

  moduleCounter += 1
  const page = await import(`${pathToFileURL(modulePath).href}?case=${moduleCounter}`)
  return { page, state }
}

async function openPage(page, state, id) {
  state.onLoad({ id })
  assert.equal(typeof state.onShow, 'function', 'booking detail should register an onShow lifecycle handler')
  assert.equal(typeof state.onHide, 'function', 'booking detail should register an onHide lifecycle handler')
  state.onShow()
  await Promise.resolve()
  await Promise.resolve()
}

try {
  for (const invalidId of ['', '   ', 'abc', '1.5', '-1', '+1', '%', '%E0%A4%A', '*']) {
    const { page, state } = await createHarness()
    state.token = 'token-a'
    await openPage(page, state, invalidId)
    assert.equal(state.listCalls, 0, `invalid route ID ${JSON.stringify(invalidId)} must not call the API`)
    assert.equal(page.notFound.value, true, `invalid route ID ${JSON.stringify(invalidId)} should show not found`)
    assert.equal(page.booking.value, null)
  }

  {
    const { page, state } = await createHarness()
    state.token = 'token-a'
    state.cachedRecord = { id: 42, kind: 'course', phone: '13800138000' }
    await openPage(page, state, '%34%32')
    assert.deepEqual(state.sessionReads, [{ bookingId: '42', token: 'token-a' }])
    assert.equal(state.listCalls, 0, 'a valid token-bound session should avoid the list fallback')
    assert.equal(page.booking.value.id, 42)
  }

  {
    const { page, state } = await createHarness()
    state.token = 'token-a'
    state.listBookingsApi = async () => {
      state.listCalls += 1
      return { items: [{ id: 7, phone: '13800138000', message: 'A 的隐私留言' }] }
    }

    await openPage(page, state, '7')
    assert.equal(state.listCalls, 1, 'the first onShow after onLoad must not duplicate the initial request')
    assert.equal(page.booking.value.phone, '13800138000')

    const clearsBeforeHide = state.sessionClearCalls
    state.onHide()
    assert.equal(page.booking.value, null, 'onHide should immediately remove the complete phone and message from visible refs')
    assert.equal(page.loading.value, false, 'onHide should stop exposing a stale loading state')
    assert.equal(state.sessionClearCalls, clearsBeforeHide + 1, 'onHide should immediately clear token-bound booking session data')

    state.token = 'token-b'
    state.listBookingsApi = async () => {
      state.listCalls += 1
      return { items: [{ id: '7', phone: '13900139000', message: 'B 的新留言' }] }
    }
    state.onShow()
    await Promise.resolve()
    await Promise.resolve()

    assert.equal(state.listCalls, 2, 'returning from a hidden page should reload under the current token')
    assert.equal(page.booking.value.phone, '13900139000', 'returning after a token change must show only the new token detail')
    assert.equal(page.booking.value.message, 'B 的新留言')
    assert.equal(state.toasts.length, 0)
    assert.equal(state.switches.length, 0)
  }

  {
    const { page, state } = await createHarness()
    const oldPending = deferred()
    state.token = 'token-a'
    state.listBookingsApi = () => {
      state.listCalls += 1
      return oldPending.promise
    }
    state.onLoad({ id: '7' })
    assert.equal(typeof state.onShow, 'function')
    assert.equal(typeof state.onHide, 'function')
    state.onShow()
    state.onHide()

    state.token = 'token-b'
    state.listBookingsApi = async () => {
      state.listCalls += 1
      return { items: [{ id: 7, phone: 'new-token' }] }
    }
    state.onShow()
    await Promise.resolve()
    await Promise.resolve()
    oldPending.resolve({ items: [{ id: 7, phone: 'old-token' }] })
    await Promise.resolve()
    await Promise.resolve()

    assert.equal(page.booking.value.phone, 'new-token', 'a request invalidated by onHide must not overwrite the reloaded token detail')
    assert.equal(state.token, 'token-b')
  }

  {
    const { page, state } = await createHarness()
    state.token = 'token-a'
    const items = Array.from({ length: 51 }, (_, index) => ({ id: index + 1 }))
    state.listBookingsApi = async () => {
      state.listCalls += 1
      return { items }
    }
    await openPage(page, state, '42')
    assert.equal(page.booking.value.id, 42, 'numeric API IDs should match normalized string route IDs')

    await openPage(page, state, '51')
    assert.equal(page.booking.value, null, 'records outside the latest 50 should not be exposed')
    assert.equal(page.notFound.value, true)
  }

  {
    const { page, state } = await createHarness()
    await openPage(page, state, '7')
    assert.equal(state.listCalls, 0, 'missing auth must not call the booking API')
    assert.equal(state.clearTokenCalls, 1)
    assert.equal(state.sessionClearCalls, 1)
    assert.equal(state.toasts.length, 1)
    assert.equal(state.switches.length, 1)
  }

  for (const statusCode of [401, 403]) {
    const { page, state } = await createHarness()
    state.token = 'token-a'
    state.listBookingsApi = async () => {
      state.listCalls += 1
      state.token = ''
      throw Object.assign(new Error('Unauthorized'), {
        authExpired: true,
        requestToken: 'token-a',
        statusCode,
      })
    }
    await openPage(page, state, '7')
    assert.equal(state.clearTokenCalls, 1, `current ${statusCode} should centralize auth cleanup`)
    assert.equal(state.sessionClearCalls, 1, `current ${statusCode} should clear booking session data`)
    assert.equal(state.toasts.length, 1, `current ${statusCode} should show one Toast`)
    assert.equal(state.switches.length, 1, `current ${statusCode} should redirect once`)
    page.retryLoad()
    assert.equal(state.toasts.length, 1, `current ${statusCode} retry must not duplicate the Toast while redirecting`)
  }

  {
    const { page, state } = await createHarness()
    const pending = deferred()
    state.token = 'token-a'
    state.listBookingsApi = () => {
      state.listCalls += 1
      return pending.promise
    }
    state.onLoad({ id: '7' })
    state.token = 'token-b'
    pending.resolve({ items: [{ id: 7, phone: 'old-user' }] })
    await Promise.resolve()
    await Promise.resolve()
    assert.equal(state.token, 'token-b', 'an old successful response must not clear a newer token')
    assert.equal(page.booking.value, null, 'an old successful response must not expose old-user detail')
    assert.equal(state.toasts.length, 0)
    assert.equal(state.switches.length, 0)
  }

  {
    const { page, state } = await createHarness()
    const pending = deferred()
    state.token = 'token-a'
    state.listBookingsApi = () => {
      state.listCalls += 1
      return pending.promise
    }
    state.onLoad({ id: '7' })
    state.token = 'token-b'
    pending.reject(Object.assign(new Error('Unauthorized'), {
      authExpired: true,
      requestToken: 'token-a',
      statusCode: 401,
    }))
    await Promise.resolve()
    await Promise.resolve()
    assert.equal(state.token, 'token-b', 'an old 401 must not clear a newer token')
    assert.equal(state.toasts.length, 0, 'an old 401 must not show an auth-expired Toast')
    assert.equal(state.switches.length, 0, 'an old 401 must not redirect the newer session')
  }

  {
    const { page, state } = await createHarness()
    const oldPending = deferred()
    state.token = 'token-a'
    let requestIndex = 0
    state.listBookingsApi = () => {
      state.listCalls += 1
      requestIndex += 1
      return requestIndex === 1 ? oldPending.promise : Promise.resolve({ items: [{ id: 8, contactName: '新页面' }] })
    }
    state.onLoad({ id: '7' })
    await openPage(page, state, '8')
    oldPending.resolve({ items: [{ id: 7, contactName: '旧页面' }] })
    await Promise.resolve()
    await Promise.resolve()
    assert.equal(page.booking.value.id, 8, 'a response for an old route ID must not replace the current detail')
  }

  {
    const { page, state } = await createHarness()
    const pending = deferred()
    state.token = 'token-a'
    state.listBookingsApi = () => {
      state.listCalls += 1
      return pending.promise
    }
    state.onLoad({ id: '7' })
    state.onUnload()
    pending.resolve({ items: [{ id: 7 }] })
    await Promise.resolve()
    await Promise.resolve()
    assert.equal(page.booking.value, null, 'an unloaded page must ignore its pending response')
    assert.ok(state.sessionClearCalls >= 1, 'unload should clear booking session data')
  }

  console.log('booking detail session tests passed')
} finally {
  delete globalThis.__bookingDetailHarness
  delete globalThis.uni
  await rm(dir, { force: true, recursive: true })
}
