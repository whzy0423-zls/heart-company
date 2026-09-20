import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-content-asset-'))
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
