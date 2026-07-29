import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const source = await readFile(new URL('./profile.vue', import.meta.url), 'utf8')
const profileEditSource = await readFile(new URL('../profile-edit/profile-edit.vue', import.meta.url), 'utf8')

for (const [pageName, pageSource] of [
  ['profile', source],
  ['profile edit', profileEditSource],
]) {
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
    assert.ok(pageSource.includes(`var(${token})`), `${pageName} page should use ${token}`)
  }
  assert.doesNotMatch(
    pageSource,
    /#(?:172554|4338ca|7c3aed|4f46e5|3730a3|ddd6fe|ede9fe|f5f3ff|eef2ff)/i,
    `${pageName} page should not keep the old full-page violet theme`,
  )
}

assert.match(source, /<view class="wrap profile page-stack ios-page ios-safe-bottom">/, 'profile should use the shared page shell')
assert.match(profileEditSource, /<view class="wrap profile-edit-page page-stack ios-page ios-safe-bottom">/, 'profile edit should use the same horizontal page shell')
assert.match(source, /v-if="!logged"/, 'profile login state should remain available')
assert.match(source, /v-if="profileLoading"/, 'profile record loading state should remain available')
assert.match(source, /v-else-if="recordsError"/, 'profile record error state should remain available')
assert.match(source, /v-else-if="records\.length === 0"/, 'profile record empty state should remain available')
assert.match(source, /@click="loadAll">重试/, 'profile record errors should keep a retry action')
assert.match(profileEditSource, /v-if="profileLoading"/, 'profile edit loading state should remain available')
assert.match(profileEditSource, /v-else-if="loadError"/, 'profile edit error state should remain available')
assert.match(profileEditSource, /@click="loadProfile">重新加载/, 'profile edit errors should keep a retry action')
assert.match(profileEditSource, /\.profile-save\s*\{[\s\S]*?min-height:\s*88rpx/, 'profile save should keep a full-size touch target')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1]
assert.ok(script, 'profile page should expose a script setup block')

const executableScript = script.replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*$/gm, '')
const dir = await mkdtemp(join(tmpdir(), 'nx-profile-session-'))
const modulePath = join(dir, 'profile-session.mjs')

const harnessPrelude = `
const ref = (value) => ({ value })
const computed = (getter) => ({ get value() { return getter() } })
const onShow = (handler) => { globalThis.__profileHarness.onShow = handler }
const TYPES_INFO = {}
const ensureLogin = () => globalThis.__profileHarness.ensureLogin()
const getToken = () => globalThis.__profileHarness.token
const clearToken = () => globalThis.__profileHarness.clearToken()
const hiddenCount = (items) => Math.max(0, items.length - 3)
const previewItems = (items) => items.slice(0, 3)
const clearBookingSession = () => globalThis.__profileHarness.clearBookingSession()
const userErrorMessage = (error, fallback) => error?.message || fallback
const getUserInfoApi = () => globalThis.__profileHarness.getUserInfoApi()
const listTestRecordsApi = () => globalThis.__profileHarness.listTestRecordsApi()
const listBookingsApi = () => globalThis.__profileHarness.listBookingsApi()
`

await writeFile(
  modulePath,
  `${harnessPrelude}\n${executableScript}\nexport { logged, user, records, bookings, recordsError, bookingsError, profileLoading, loadAll }\n`,
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

function authError(token, statusCode = 401) {
  return Object.assign(new Error('Unauthorized'), {
    authExpired: true,
    requestToken: token,
    statusCode,
  })
}

async function createHarness() {
  const state = {
    token: '',
    clearTokenCalls: 0,
    sessionClearCalls: 0,
    toasts: [],
    navigations: [],
    ensureLogin: async () => {},
    getUserInfoApi: async () => ({ nickname: '当前用户' }),
    listTestRecordsApi: async () => ({ items: [] }),
    listBookingsApi: async () => ({ items: [] }),
    clearToken() {
      this.clearTokenCalls += 1
      this.token = ''
    },
    clearBookingSession() {
      this.sessionClearCalls += 1
    },
  }

  globalThis.__profileHarness = state
  globalThis.uni = {
    showToast(options) { state.toasts.push(options) },
    navigateTo(options) { state.navigations.push(options) },
  }

  moduleCounter += 1
  const page = await import(`${pathToFileURL(modulePath).href}?case=${moduleCounter}`)
  page.logged.value = true
  return { page, state }
}

try {
  {
    const { page, state } = await createHarness()
    const pendingUser = deferred()
    state.token = 'token-a'
    state.getUserInfoApi = () => pendingUser.promise

    const loadPromise = page.loadAll()
    state.token = 'token-b'
    pendingUser.resolve({ nickname: '旧用户' })
    await loadPromise

    assert.equal(state.token, 'token-b', 'an old successful profile response must not clear a newer token')
    assert.equal(page.user.value, null, 'an old successful profile response must not expose old user data')
    assert.deepEqual(page.records.value, [], 'an old successful profile response must not expose old test records')
    assert.deepEqual(page.bookings.value, [], 'an old successful profile response must not expose old bookings')
    assert.equal(state.sessionClearCalls, 1, 'an old successful profile response should clear the old booking session')
    assert.equal(state.clearTokenCalls, 0, 'an old successful profile response must not clear auth')
    assert.equal(state.toasts.length, 0, 'an old successful profile response must not show auth feedback')
  }

  for (const statusCode of [401, 403]) {
    const { page, state } = await createHarness()
    const pendingUser = deferred()
    state.token = 'token-a'
    state.getUserInfoApi = () => pendingUser.promise

    const loadPromise = page.loadAll()
    state.token = 'token-b'
    pendingUser.reject(authError('token-a', statusCode))
    await loadPromise

    assert.equal(state.token, 'token-b', `an old profile ${statusCode} must not clear a newer token`)
    assert.equal(page.user.value, null, `an old profile ${statusCode} should discard old user data`)
    assert.equal(state.sessionClearCalls, 1, `an old profile ${statusCode} should clear the old booking session`)
    assert.equal(state.clearTokenCalls, 0, `an old profile ${statusCode} must not clear auth`)
    assert.equal(state.toasts.length, 0, `an old profile ${statusCode} must not show auth feedback`)
  }

  for (const statusCode of [401, 403]) {
    const { page, state } = await createHarness()
    const pendingUser = deferred()
    state.token = 'token-a'
    state.getUserInfoApi = () => pendingUser.promise

    const loadPromise = page.loadAll()
    state.token = ''
    pendingUser.reject(authError('token-a', statusCode))
    await loadPromise

    assert.equal(page.logged.value, false, `a current profile ${statusCode} should reset login state`)
    assert.equal(state.clearTokenCalls, 1, `a current profile ${statusCode} should centralize token cleanup`)
    assert.equal(state.sessionClearCalls, 1, `a current profile ${statusCode} should clear booking session data`)
    assert.equal(state.toasts.length, 1, `a current profile ${statusCode} should show one auth-expired Toast`)
  }

  for (const { failingRequest, failure } of [
    { failingRequest: 'records', failure: authError('token-a', 401) },
    { failingRequest: 'bookings', failure: authError('token-a', 403) },
    { failingRequest: 'bookings', failure: new Error('Network unavailable') },
  ]) {
    const { page, state } = await createHarness()
    const pendingRecords = deferred()
    const pendingBookings = deferred()
    state.token = 'token-a'
    state.getUserInfoApi = async () => ({ nickname: '旧用户' })
    state.listTestRecordsApi = () => pendingRecords.promise
    state.listBookingsApi = () => pendingBookings.promise

    const loadPromise = page.loadAll()
    await Promise.resolve()
    state.token = 'token-b'
    if (failingRequest === 'records') {
      pendingRecords.reject(failure)
      pendingBookings.resolve({ items: [{ id: 1 }] })
    } else {
      pendingRecords.resolve({ items: [{ id: 1 }] })
      pendingBookings.reject(failure)
    }
    await loadPromise

    assert.equal(state.token, 'token-b', `an old ${failingRequest} failure must not clear a newer token`)
    assert.equal(page.user.value, null, `an old ${failingRequest} failure should discard the old profile snapshot`)
    assert.deepEqual(page.records.value, [], `an old ${failingRequest} failure must not expose test record results`)
    assert.deepEqual(page.bookings.value, [], `an old ${failingRequest} failure must not expose booking results`)
    assert.equal(state.sessionClearCalls, 1, `an old ${failingRequest} failure should clear the old booking session`)
    assert.equal(state.clearTokenCalls, 0, `an old ${failingRequest} failure must not clear auth`)
    assert.equal(state.toasts.length, 0, `an old ${failingRequest} failure must not show auth feedback`)
  }

  for (const failingRequest of ['records', 'bookings']) {
    for (const statusCode of [401, 403]) {
      const { page, state } = await createHarness()
      const pendingRecords = deferred()
      const pendingBookings = deferred()
      state.token = 'token-a'
      state.listTestRecordsApi = () => pendingRecords.promise
      state.listBookingsApi = () => pendingBookings.promise

      const loadPromise = page.loadAll()
      await Promise.resolve()
      state.token = ''
      if (failingRequest === 'records') {
        pendingRecords.reject(authError('token-a', statusCode))
        pendingBookings.resolve({ items: [] })
      } else {
        pendingRecords.resolve({ items: [] })
        pendingBookings.reject(authError('token-a', statusCode))
      }
      await loadPromise

      assert.equal(page.logged.value, false, `a current ${failingRequest} ${statusCode} should reset login state`)
      assert.equal(state.clearTokenCalls, 1, `a current ${failingRequest} ${statusCode} should centralize token cleanup`)
      assert.equal(state.sessionClearCalls, 1, `a current ${failingRequest} ${statusCode} should clear booking session data`)
      assert.equal(state.toasts.length, 1, `a current ${failingRequest} ${statusCode} should show one auth-expired Toast`)
    }
  }

  console.log('profile session tests passed')
} finally {
  delete globalThis.__profileHarness
  delete globalThis.uni
  await rm(dir, { force: true, recursive: true })
}
