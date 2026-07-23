import assert from 'node:assert/strict'
import { copyFile, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-config-'))
const modulePath = join(dir, 'config.mjs')
await copyFile(new URL('./config.js', import.meta.url), modulePath)
const { resolveApiBase } = await import(`file://${modulePath}`)

assert.equal(
  resolveApiBase({ env: { DEV: true, VITE_API_BASE: '' } }),
  'https://xn--9iq9az5uo8fz16d.com/api',
)

assert.equal(
  resolveApiBase({ env: { DEV: false, VITE_API_BASE: '' } }),
  'https://xn--9iq9az5uo8fz16d.com/api',
)

assert.throws(
  () => resolveApiBase({ env: { DEV: false, VITE_API_BASE: 'https://api.example.com/api' } }),
  /real HTTPS API URL/,
)

assert.throws(
  () => resolveApiBase({ env: { DEV: false, VITE_API_BASE: 'http://api.nine-xing.com/api' } }),
  /real HTTPS API URL/,
)

for (const loopbackApiBase of [
  'https://localhost/api',
  'https://127.0.0.1/api',
  'https://[::1]/api',
]) {
  assert.throws(
    () => resolveApiBase({ env: { DEV: false, VITE_API_BASE: loopbackApiBase } }),
    /real HTTPS API URL/,
  )
}

assert.equal(
  resolveApiBase({ env: { DEV: false, VITE_API_BASE: 'https://xn--9iq9az5uo8fz16d.com/api/' } }),
  'https://xn--9iq9az5uo8fz16d.com/api',
)

assert.equal(
  resolveApiBase({ env: { DEV: false, VITE_API_BASE: 'https://alternate.example.net/api' } }),
  'https://alternate.example.net/api',
)

console.log('config tests passed')
await rm(dir, { force: true, recursive: true })
