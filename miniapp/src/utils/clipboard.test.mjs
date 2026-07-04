import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-clipboard-'))
const modulePath = join(dir, 'clipboard.mjs')
const source = await readFile(new URL('./clipboard.js', import.meta.url), 'utf8')
await writeFile(modulePath, source)

const { copyText } = await import(`file://${modulePath}`)

let copied = ''
let toasts = []
const success = await copyText('  九型答案  ', {
  clipboard: ({ data, success }) => {
    copied = data
    success()
  },
  toast: (options) => toasts.push(options),
})
assert.equal(success, true)
assert.equal(copied, '九型答案')
assert.deepEqual(toasts.at(-1), { title: '已复制答案', icon: 'success' })

toasts = []
const failed = await copyText('失败路径', {
  clipboard: ({ fail }) => fail(),
  toast: (options) => toasts.push(options),
})
assert.equal(failed, false)
assert.deepEqual(toasts.at(-1), { title: '复制失败，请重试', icon: 'none' })

toasts = []
const empty = await copyText('   ', {
  clipboard: () => { throw new Error('clipboard should not be called for empty text') },
  toast: (options) => toasts.push(options),
})
assert.equal(empty, false)
assert.deepEqual(toasts.at(-1), { title: '没有可复制内容', icon: 'none' })

console.log('clipboard tests passed')
await rm(dir, { force: true, recursive: true })
