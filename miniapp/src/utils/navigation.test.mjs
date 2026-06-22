import assert from 'node:assert/strict'
import { copyFile, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-navigation-'))
const modulePath = join(dir, 'navigation.mjs')
await copyFile(new URL('./navigation.js', import.meta.url), modulePath)
const { CHAT_TAB_URL, openChatPage } = await import(`file://${modulePath}`)

const calls = []
openChatPage({
  navigateTo(options) {
    calls.push(['navigateTo', options])
  },
  switchTab(options) {
    calls.push(['switchTab', options])
  },
})

assert.equal(CHAT_TAB_URL, '/pages/chat/chat')
assert.deepEqual(calls, [['switchTab', { url: '/pages/chat/chat' }]])

console.log('navigation tests passed')
await rm(dir, { force: true, recursive: true })
