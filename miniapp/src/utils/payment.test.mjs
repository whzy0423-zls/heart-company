import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(resolve(__dirname, './payment.js'), 'utf8')

assert.match(
  source,
  /url:\s*['"]\/miniapp\/report\/dev-pay['"]/,
  'dev payment simulation must use the authenticated miniapp endpoint',
)
assert.doesNotMatch(
  source,
  /url:\s*['"]\/pay\/notify['"]/,
  'miniapp must not call the public wxpay callback endpoint',
)

console.log('payment dev simulation tests passed')
