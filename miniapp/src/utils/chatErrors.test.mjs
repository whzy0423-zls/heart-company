import assert from 'node:assert/strict'
import { copyFile, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-chat-errors-'))
const modulePath = join(dir, 'chatErrors.mjs')
await copyFile(new URL('./chatErrors.js', import.meta.url), modulePath)
const { chatErrorMessage } = await import(`file://${modulePath}`)

assert.equal(
  chatErrorMessage(new Error('微信登录未返回 code，请稍后重试')),
  '微信登录没有拿到授权 code，请稍后重试，或重新打开小程序。',
)

assert.equal(
  chatErrorMessage(Object.assign(new Error('Forbidden'), { statusCode: 403 })),
  '登录状态已失效，请重新进入页面后再试。',
)

assert.equal(
  chatErrorMessage(Object.assign(new Error('提问太频繁了，请稍后再试'), { statusCode: 429 })),
  '提问有点太频繁了，稍等一分钟再继续问。',
)

assert.equal(
  chatErrorMessage(new Error('context deadline exceeded')),
  '本次回答生成超时了，可以稍后再试，或把问题问得更具体一点。',
)

assert.equal(
  chatErrorMessage(new Error('timeout')),
  '刚才没有连接上对话服务，请稍后再试，或换个更具体的问题。',
)

console.log('chat error tests passed')
await rm(dir, { force: true, recursive: true })
