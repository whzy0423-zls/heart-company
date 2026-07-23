import assert from 'node:assert/strict'
import { dirname, resolve } from 'node:path'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const scriptPath = resolve(dirname(fileURLToPath(import.meta.url)), 'verify-production-api-base.mjs')

function verify(apiBase) {
  return spawnSync(process.execPath, [scriptPath], {
    cwd: resolve(dirname(scriptPath), '..'),
    env: { ...process.env, VITE_API_BASE: apiBase },
    encoding: 'utf8',
  })
}

assert.equal(verify('https://alternate.example.net/api').status, 0)
assert.equal(verify('https://[::ffff:8.8.8.8]/api').status, 0)

for (const invalidApiBase of [
  'https://example.com/api',
  'https://yourdomain.com/api',
  'https://localhost./api',
  'https://service.local./api',
  'https://api.example.com./api',
  'https://[::]/api',
  'https://[::1]/api',
  'https://[::ffff:127.0.0.1]/api',
  'https://[::ffff:7f00:1]/api',
  'https://[::ffff:10.0.0.1]/api',
  'https://[::ffff:a00:1]/api',
  'https://[fc00::1]/api',
  'https://[fd12:3456:789a::1]/api',
  'https://[fe80::1]/api',
]) {
  const result = verify(invalidApiBase)
  assert.notEqual(result.status, 0, `${invalidApiBase} must be rejected`)
  assert.match(result.stderr, /must not use placeholder or local development hosts/)
}

const invalidNumericHost = verify('https://08/api')
assert.notEqual(invalidNumericHost.status, 0)
assert.match(invalidNumericHost.stderr, /must be a valid HTTPS URL/)

console.log('production API base verifier tests passed')
