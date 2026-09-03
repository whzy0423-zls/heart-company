import assert from 'node:assert/strict'
import test from 'node:test'

import {
  AppReleaseAPIError,
  buildLatestAppReleaseDownloadURL,
  getLatestAppRelease,
} from './appRelease.js'

function jsonResponse(body, { ok = true, status = 200 } = {}) {
  return {
    ok,
    status,
    async json() {
      return body
    },
  }
}

test('loads the latest Android release through the public response envelope', async () => {
  const signal = new AbortController().signal
  const release = {
    available: true,
    versionName: '1.4.0',
    versionCode: 10400,
    publishedAt: '2026-07-20T08:30:00Z',
    fileSize: 15728640,
    sha256: 'a'.repeat(64),
    releaseNotes: '新增课程离线阅读。',
  }
  let request

  const result = await getLatestAppRelease({
    apiBase: '/api',
    signal,
    fetchImpl: async (url, options) => {
      request = { url, options }
      return jsonResponse({ code: 0, data: release })
    },
  })

  assert.deepEqual(result, release)
  assert.equal(request.url, '/api/public/app-release/latest?platform=android')
  assert.equal(request.options.headers.Accept, 'application/json')
  assert.equal(request.options.signal, signal)
})

test('returns the public unavailable state without converting it to an error', async () => {
  const result = await getLatestAppRelease({
    fetchImpl: async () => jsonResponse({ code: 0, data: { available: false } }),
  })

  assert.deepEqual(result, { available: false })
})

test('uses a safe error for non-2xx responses', async () => {
  await assert.rejects(
    getLatestAppRelease({
      fetchImpl: async () => jsonResponse(
        { code: 500, message: 'open /srv/private/app-release.apk: permission denied' },
        { ok: false, status: 500 },
      ),
    }),
    (error) => {
      assert.ok(error instanceof AppReleaseAPIError)
      assert.equal(error.status, 500)
      assert.doesNotMatch(error.message, /srv|permission denied/i)
      return true
    },
  )
})

test('rejects a non-success response envelope even when HTTP succeeds', async () => {
  await assert.rejects(
    getLatestAppRelease({
      fetchImpl: async () => jsonResponse({ code: 1001, message: 'internal detail' }),
    }),
    (error) => error instanceof AppReleaseAPIError && error.status === 200,
  )
})

test('preserves AbortError unchanged', async () => {
  const abortError = new DOMException('The operation was aborted', 'AbortError')

  await assert.rejects(
    getLatestAppRelease({ fetchImpl: async () => { throw abortError } }),
    (error) => error === abortError,
  )
})

test('preserves AbortError raised while reading the response body', async () => {
  const abortError = new DOMException('The operation was aborted', 'AbortError')

  await assert.rejects(
    getLatestAppRelease({
      fetchImpl: async () => ({
        ok: true,
        status: 200,
        async json() {
          throw abortError
        },
      }),
    }),
    (error) => error === abortError,
  )
})

test('wraps network failures in a safe API error', async () => {
  await assert.rejects(
    getLatestAppRelease({
      fetchImpl: async () => { throw new Error('connect ECONNREFUSED 10.0.0.8:5432') },
    }),
    (error) => {
      assert.ok(error instanceof AppReleaseAPIError)
      assert.equal(error.status, 0)
      assert.doesNotMatch(error.message, /ECONNREFUSED|10\.0\.0\.8/)
      return true
    },
  )
})

test('retains a 503 status for a missing published package', async () => {
  await assert.rejects(
    getLatestAppRelease({
      fetchImpl: async () => jsonResponse({ code: 503 }, { ok: false, status: 503 }),
    }),
    (error) => error instanceof AppReleaseAPIError && error.status === 503,
  )
})

test('builds latest download URLs for relative and absolute API bases', () => {
  assert.equal(
    buildLatestAppReleaseDownloadURL('/api'),
    '/api/public/app-release/download?platform=android',
  )
  assert.equal(
    buildLatestAppReleaseDownloadURL('https://api.example.com/api'),
    'https://api.example.com/api/public/app-release/download?platform=android',
  )
})
