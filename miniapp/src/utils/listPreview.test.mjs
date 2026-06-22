import assert from 'node:assert/strict'
import { copyFile, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-list-preview-'))
const modulePath = join(dir, 'listPreview.mjs')
await copyFile(new URL('./listPreview.js', import.meta.url), modulePath)
const { hiddenCount, previewItems } = await import(`file://${modulePath}`)

const items = [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }]

assert.deepEqual(previewItems(items), [{ id: 1 }, { id: 2 }, { id: 3 }])
assert.deepEqual(previewItems(items, 2), [{ id: 1 }, { id: 2 }])
assert.deepEqual(previewItems(null), [])

assert.equal(hiddenCount(items), 1)
assert.equal(hiddenCount(items, 2), 2)
assert.equal(hiddenCount(items, 8), 0)
assert.equal(hiddenCount(undefined), 0)

console.log('list preview tests passed')
await rm(dir, { force: true, recursive: true })
