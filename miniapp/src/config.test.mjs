import assert from 'node:assert/strict'
import { copyFile, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-config-'))
const modulePath = join(dir, 'config.mjs')
await copyFile(new URL('./config.js', import.meta.url), modulePath)
await copyFile(new URL('./apiBaseValidation.mjs', import.meta.url), join(dir, 'apiBaseValidation.mjs'))
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
  'https://127.1/api',
  'https://2130706433/api',
  'https://0x7f000001/api',
  'https://[::1]/api',
  'https://example.com/api',
  'https://yourdomain.com/api',
  'https://localhost./api',
  'https://service.local./api',
  'https://api.example.com./api',
  'https://[::]/api',
  'https://[::ffff:127.0.0.1]/api',
  'https://[::ffff:7f00:1]/api',
  'https://[::ffff:10.0.0.1]/api',
  'https://[::ffff:a00:1]/api',
  'https://[fc00::1]/api',
  'https://[fd12:3456:789a::1]/api',
  'https://[fe80::1]/api',
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

assert.equal(
  resolveApiBase({ env: { DEV: false, VITE_API_BASE: 'https://[::ffff:8.8.8.8]/api' } }),
  'https://[::ffff:8.8.8.8]/api',
)

console.log('config tests passed')
await rm(dir, { force: true, recursive: true })
