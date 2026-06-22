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
  'http://localhost:8080/api',
)

assert.equal(
  resolveApiBase({ env: { DEV: false, VITE_API_BASE: '' } }),
  'https://api.example.com/api',
)

assert.equal(
  resolveApiBase({ env: { DEV: false, VITE_API_BASE: 'https://api.nine-xing.com/api' } }),
  'https://api.nine-xing.com/api',
)

console.log('config tests passed')
await rm(dir, { force: true, recursive: true })
