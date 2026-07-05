import assert from 'node:assert/strict'
import { copyFile, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-result-persona-'))
const modulePath = join(dir, 'resultPersona.mjs')
await copyFile(new URL('./resultPersona.js', import.meta.url), modulePath)
const { resultPersonaText } = await import(`file://${modulePath}`)

const profile = {
  base: '通用画像',
  male: '男版画像',
  female: '女版画像',
}

assert.equal(resultPersonaText(profile, 'male'), '男版画像')
assert.equal(resultPersonaText(profile, 'female'), '女版画像')
assert.equal(
  resultPersonaText(profile, null),
  '通用画像',
  'missing gender should use the neutral base persona instead of implicitly using female copy',
)
assert.equal(resultPersonaText(profile, 'unknown'), '通用画像')
assert.equal(resultPersonaText(null, 'male'), '')

console.log('result persona tests passed')
await rm(dir, { force: true, recursive: true })
