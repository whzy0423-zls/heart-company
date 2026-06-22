import assert from 'node:assert/strict'
import { copyFile, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-chat-state-'))
const modulePath = join(dir, 'chatState.mjs')
await copyFile(new URL('./chatState.js', import.meta.url), modulePath)
const {
  buildRecentHistory,
  chatStatusText,
  limitMessages,
  normalizeSources,
  restoreMessages,
  serializeMessages,
} = await import(`file://${modulePath}`)

assert.deepEqual(
  buildRecentHistory([
    { role: 'assistant', content: '欢迎语', localOnly: true },
    { role: 'user', content: '  第一个问题  ' },
    { role: 'assistant', content: 'a'.repeat(260) },
    { role: 'system', content: 'skip' },
  ]),
  [
    { role: 'user', content: '第一个问题' },
    { role: 'assistant', content: `${'a'.repeat(220)}...` },
  ],
)

assert.equal(limitMessages(Array.from({ length: 34 }, (_, i) => ({ id: String(i) }))).length, 28)

assert.deepEqual(
  serializeMessages([
    { id: 'welcome', role: 'assistant', content: '欢迎语', localOnly: true },
    { id: 'u1', role: 'user', content: '问题', sources: [] },
    { id: 'a1', role: 'assistant', content: '回答', sources: [{ id: 's1', title: '资料', snippet: '摘要' }] },
    { id: 'bad', role: 'system', content: 'skip' },
  ]),
  [
    { id: 'u1', role: 'user', content: '问题', sources: [] },
    { id: 'a1', role: 'assistant', content: '回答', sources: [{ id: 's1', title: '资料', snippet: '摘要' }] },
  ],
)

assert.deepEqual(
  restoreMessages([
    { id: 'u1', role: 'user', content: '问题' },
    { id: 'a1', role: 'assistant', content: '回答', sources: [{ id: 's1', title: ' 资料 ', snippet: '摘要' }] },
    { id: 'empty', role: 'assistant', content: '   ' },
  ]),
  [
    { id: 'u1', role: 'user', content: '问题', sources: [] },
    { id: 'a1', role: 'assistant', content: '回答', sources: [{ id: 's1', title: '资料', snippet: '摘要' }] },
  ],
)

assert.deepEqual(
  normalizeSources([
    { id: '1', title: '  标题  ', snippet: 'b'.repeat(120) },
    { id: '2', title: '', snippet: '无标题不展示' },
    { id: '3', title: '第三条', snippet: '不超过两条' },
  ]),
  [
    { id: '1', title: '标题', snippet: `${'b'.repeat(84)}...` },
    { id: '3', title: '第三条', snippet: '不超过两条' },
  ],
)

assert.equal(chatStatusText({ sending: true }), '正在检索知识库')
assert.equal(chatStatusText({ lastSources: [{ id: 's1' }] }), '已命中 1 条资料')
assert.equal(chatStatusText({ lastSources: [{ id: 's1' }, { id: 's2' }] }), '已命中 2 条资料')
assert.equal(chatStatusText({ hasConversation: true, lastSources: [] }), '可继续追问')
assert.equal(chatStatusText({ hasConversation: false, lastSources: [] }), 'RAG 知识检索已开启')

console.log('chat state tests passed')
await rm(dir, { force: true, recursive: true })
