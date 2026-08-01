import assert from 'node:assert/strict'
import { copyFile, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-type-theme-'))
const modulePath = join(dir, 'typeTheme.mjs')
await copyFile(new URL('./typeTheme.js', import.meta.url), modulePath)
const { TYPE_THEME, typeTheme } = await import(`file://${modulePath}`)

const expectedAccents = {
  1: '#315BEA',
  2: '#C9472D',
  3: '#B86A12',
  4: '#8065B5',
  5: '#347B62',
  6: '#42658D',
  7: '#C47B18',
  8: '#A43D35',
  9: '#5D7766',
}

for (const [typeId, accent] of Object.entries(expectedAccents)) {
  assert.equal(TYPE_THEME[typeId].accent, accent, `type ${typeId} should use its specified accent`)
}

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
