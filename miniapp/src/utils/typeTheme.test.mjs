import assert from 'node:assert/strict'
import { copyFile, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-type-theme-'))
const modulePath = join(dir, 'typeTheme.mjs')
await copyFile(new URL('./typeTheme.js', import.meta.url), modulePath)
const { TYPE_THEME, typeTheme } = await import(`file://${modulePath}`)

assert.equal(TYPE_THEME[1].accent, '#315BEA')
assert.equal(TYPE_THEME[6].accent, '#42658D')
assert.equal(TYPE_THEME[9].accent, '#5D7766')

for (let typeId = 1; typeId <= 9; typeId += 1) {
  const theme = typeTheme(typeId)
  assert.deepEqual(Object.keys(theme).sort(), ['accent', 'ink', 'soft'])
  assert.match(theme.accent, /^#[0-9A-F]{6}$/i)
  assert.match(theme.soft, /^#[0-9A-F]{6}$/i)
  assert.match(theme.ink, /^#[0-9A-F]{6}$/i)
}

const fallback = typeTheme('invalid')
assert.equal(fallback.accent, '#68727C')
assert.equal(typeTheme(0), fallback)
assert.equal(typeTheme(10), fallback)
assert.equal(typeTheme(null), fallback)
assert.equal(typeTheme('1'), TYPE_THEME[1], 'numeric type ids should work after route/storage serialization')

console.log('type theme tests passed')
await rm(dir, { force: true, recursive: true })
