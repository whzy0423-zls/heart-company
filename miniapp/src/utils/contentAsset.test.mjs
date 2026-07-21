import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-content-asset-'))
const modulePath = join(dir, 'contentAsset.mjs')
let source = await readFile(new URL('./contentAsset.js', import.meta.url), 'utf8')
source = source.replace("import { API_BASE } from '../config'", "const API_BASE = 'https://api.nine-xing.test/api'")
await writeFile(modulePath, source)

const { resolveContentAsset } = await import(`file://${modulePath}`)
const fallback = '/static/editorial/home-hero.webp'

assert.equal(
  resolveContentAsset('https://cdn.example.com/teacher.jpg', fallback),
  'https://cdn.example.com/teacher.jpg',
  'absolute HTTPS assets should pass through unchanged',
)
assert.equal(resolveContentAsset('HTTPS://cdn.example.com/teacher.jpg', fallback), 'HTTPS://cdn.example.com/teacher.jpg', 'HTTPS matching should be case-insensitive')
assert.equal(resolveContentAsset('/static/editorial/course-intro.webp', fallback), '/static/editorial/course-intro.webp', '/static assets should stay local')
assert.equal(resolveContentAsset('/assets/teacher-poster.jpg', fallback), 'https://api.nine-xing.test/assets/teacher-poster.jpg', '/assets should resolve against the HTTPS API origin')
assert.equal(
  resolveContentAsset('/api/public/site-uploads/teacher.jpg', fallback),
  'https://api.nine-xing.test/api/public/site-uploads/teacher.jpg',
  '/api/public/site-uploads should resolve against the HTTPS API origin without duplicating /api',
)

for (const unsafe of [
  '',
  '   ',
  'http://cdn.example.com/teacher.jpg',
  'javascript:alert(1)',
  'data:image/png;base64,AAAA',
  '//cdn.example.com/teacher.jpg',
  'teacher.jpg',
  'https://',
  '/assets/../private.jpg',
]) {
  assert.equal(resolveContentAsset(unsafe, fallback), fallback, `unsafe or malformed asset should use fallback: ${unsafe}`)
}
assert.equal(resolveContentAsset(null, ''), '', 'the caller-provided fallback should be deterministic even when empty')

console.log('content asset resolver tests passed')
await rm(dir, { force: true, recursive: true })
