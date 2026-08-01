import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-content-asset-'))
<<<<<<< HEAD
try {
  const modulePath = join(dir, 'contentAsset.mjs')
  const rawSource = await readFile(new URL('./contentAsset.js', import.meta.url), 'utf8')
  let source = rawSource
  source = source.replace(
    /import \{ API_BASE(?:, DEFAULT_API_BASE)? \} from '\.\.\/config'/,
    "const API_BASE = 'http://127.0.0.1:5320/api'; const DEFAULT_API_BASE = 'https://api.example.test/api'",
  )
  await writeFile(modulePath, source)

  const { resolveContentAsset } = await import(`file://${modulePath}`)
  const fallback = '/static/avatars/9.png'
  const originalURL = globalThis.URL
  globalThis.URL = undefined

  assert.doesNotThrow(
    () => resolveContentAsset('https://cdn.example.com/teacher.jpg', fallback),
    'asset resolution should not depend on the browser URL constructor',
  )
  assert.equal(
    resolveContentAsset('https://cdn.example.com/teacher.jpg', fallback),
    'https://cdn.example.com/teacher.jpg',
  )
  assert.equal(resolveContentAsset('/static/wheel.png', fallback), '/static/wheel.png')
  assert.equal(
    resolveContentAsset('/assets/teacher-poster.jpg', fallback),
    'https://api.example.test/assets/teacher-poster.jpg',
    'website-root assets should use the public host instead of being treated as miniapp package paths',
  )
  assert.equal(
    resolveContentAsset('/api/public/site-assets/63', fallback),
    'http://127.0.0.1:5320/api/public/site-assets/63',
  )
  assert.equal(
    resolveContentAsset('/api/public/site-uploads/teacher.jpg', fallback),
    'http://127.0.0.1:5320/api/public/site-uploads/teacher.jpg',
  )

  for (const unsafe of [
    '',
    'http://cdn.example.com/teacher.jpg',
    'javascript:alert(1)',
    'data:image/png;base64,AAAA',
    '//cdn.example.com/teacher.jpg',
    'teacher.jpg',
    'https://user@cdn.example.com/teacher.jpg',
    'https://cdn.example.com:65536/teacher.jpg',
    'https://bad..example/teacher.jpg',
    'https://cdn.example.com/%252e%252e/private.jpg',
    '/assets/../private.jpg',
    '/assets/folder%5cprivate.jpg',
    '/api/public/site-assets/%252e%252e/private.jpg',
    '/api/public/site-uploads/file%00.jpg',
    '/static/bad%encoding.jpg',
  ]) {
    assert.equal(resolveContentAsset(unsafe, fallback), fallback, unsafe)
  }

  assert.equal(
    resolveContentAsset('/assets/%E9%9F%A9%E8%80%81%E5%B8%88.jpg', fallback),
    'https://api.example.test/assets/%E9%9F%A9%E8%80%81%E5%B8%88.jpg',
  )
  assert.equal(resolveContentAsset(null, ''), '')

  const lanModulePath = join(dir, 'contentAsset-lan.mjs')
  await writeFile(
    lanModulePath,
    rawSource.replace(
      /import \{ API_BASE(?:, DEFAULT_API_BASE)? \} from '\.\.\/config'/,
      "const API_BASE = 'http://192.168.1.20:5320/api'; const DEFAULT_API_BASE = 'https://api.example.test/api'",
    ),
  )
  const { resolveContentAsset: resolveLanAsset } = await import(`file://${lanModulePath}`)
  assert.equal(
    resolveLanAsset('/api/public/site-assets/63', fallback),
    'http://192.168.1.20:5320/api/public/site-assets/63',
    'LAN device debugging should keep backend upload assets on the configured private API host',
  )
  globalThis.URL = originalURL

  console.log('content asset resolver tests passed')
} finally {
  await rm(dir, { force: true, recursive: true })
}
=======
const modulePath = join(dir, 'contentAsset.mjs')
let source = await readFile(new URL('./contentAsset.js', import.meta.url), 'utf8')
source = source.replace("import { API_BASE } from '../config'", "const API_BASE = 'https://api.nine-xing.test/api'")
await writeFile(modulePath, source)

const { resolveContentAsset } = await import(`file://${modulePath}`)
const fallback = '/static/editorial/home-hero.webp'
const originalURL = globalThis.URL
globalThis.URL = undefined

assert.doesNotThrow(
  () => resolveContentAsset('https://cdn.example.com/teacher.jpg', fallback),
  'the resolver should not depend on a global WHATWG URL implementation',
)

assert.equal(
  resolveContentAsset('https://cdn.example.com/teacher.jpg', fallback),
  'https://cdn.example.com/teacher.jpg',
  'absolute HTTPS assets should pass through unchanged',
)
assert.equal(resolveContentAsset('HTTPS://cdn.example.com/teacher.jpg', fallback), 'HTTPS://cdn.example.com/teacher.jpg', 'HTTPS matching should be case-insensitive')
assert.equal(resolveContentAsset('https://cdn.example.com:8443/teacher.jpg', fallback), 'https://cdn.example.com:8443/teacher.jpg', 'HTTPS assets should support a valid optional port')
assert.equal(resolveContentAsset('/static/editorial/course-intro.webp', fallback), '/static/editorial/course-intro.webp', '/static assets should stay local')
assert.equal(resolveContentAsset('/assets/teacher-poster.jpg', fallback), 'https://api.nine-xing.test/assets/teacher-poster.jpg', '/assets should resolve against the HTTPS API origin')
assert.equal(
  resolveContentAsset('/api/public/site-uploads/teacher.jpg', fallback),
  'https://api.nine-xing.test/api/public/site-uploads/teacher.jpg',
  '/api/public/site-uploads should resolve against the HTTPS API origin without duplicating /api',
)

let excessivelyEncodedDot = '%2e'
for (let index = 0; index < 8; index += 1) excessivelyEncodedDot = encodeURIComponent(excessivelyEncodedDot)
const protectedPrefixes = ['/static/', '/assets/', '/api/public/site-uploads/']
const encodedAttackPaths = protectedPrefixes.flatMap((prefix) => [
  `${prefix}%2e%2e/private.jpg`,
  `${prefix}%252e%252e/private.jpg`,
  `${prefix}folder%5cprivate.jpg`,
  `${prefix}folder%255cprivate.jpg`,
])

for (const unsafe of [
  '',
  '   ',
  'http://cdn.example.com/teacher.jpg',
  'javascript:alert(1)',
  'data:image/png;base64,AAAA',
  '//cdn.example.com/teacher.jpg',
  'teacher.jpg',
  'https://',
  'https:///teacher.jpg',
  'https://user:pass@cdn.example.com/teacher.jpg',
  'https://user@cdn.example.com/teacher.jpg',
  'https://cdn.example.com:0/teacher.jpg',
  'https://cdn.example.com:65536/teacher.jpg',
  'https://cdn.example.com:abc/teacher.jpg',
  'https://bad..example/teacher.jpg',
  'https://-bad.example/teacher.jpg',
  'https://bad-.example/teacher.jpg',
  'https://999.999.999.999/teacher.jpg',
  'https://cdn .example.com/teacher.jpg',
  '\nhttps://cdn.example.com/teacher.jpg',
  'https://cdn.example.com/teacher.jpg\r',
  'https://cdn.example.com\\@evil.test/teacher.jpg',
  'https://cdn.example.com/%252e%252e/private.jpg',
  'https://cdn.example.com/file%00.jpg',
  'https://cdn.example.com/bad%encoding.jpg',
  '/assets/../private.jpg',
  '/api/public/site-uploads/folder%2f..%2fprivate.jpg',
  '/api/public/site-uploads/file%00.jpg',
  '/assets/file%250a.jpg',
  '/static/file%C2%85.jpg',
  '/assets/file%25C2%2585.jpg',
  '/static/bad%encoding.jpg',
  `/assets/${excessivelyEncodedDot}.jpg`,
  ...encodedAttackPaths,
]) {
  assert.equal(resolveContentAsset(unsafe, fallback), fallback, `unsafe or malformed asset should use fallback: ${unsafe}`)
}

assert.equal(resolveContentAsset('/static/%E8%AF%BE%E7%A8%8B.webp', fallback), '/static/%E8%AF%BE%E7%A8%8B.webp', 'a valid encoded local filename should remain local')
assert.equal(
  resolveContentAsset('/assets/%E9%9F%A9%E8%80%81%E5%B8%88.jpg', fallback),
  'https://api.nine-xing.test/assets/%E9%9F%A9%E8%80%81%E5%B8%88.jpg',
  'a valid encoded asset filename should resolve normally',
)
assert.equal(
  resolveContentAsset('/api/public/site-uploads/%E8%80%81%E5%B8%88.jpg', fallback),
  'https://api.nine-xing.test/api/public/site-uploads/%E8%80%81%E5%B8%88.jpg',
  'a valid encoded upload filename should resolve normally',
)
assert.equal(resolveContentAsset(null, ''), '', 'the caller-provided fallback should be deterministic even when empty')

globalThis.URL = originalURL
console.log('content asset resolver tests passed')
await rm(dir, { force: true, recursive: true })
>>>>>>> feature/miniapp-editorial-ui
