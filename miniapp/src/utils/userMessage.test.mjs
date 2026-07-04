import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-user-message-'))
const modulePath = join(dir, 'userMessage.mjs')
const source = await readFile(new URL('./userMessage.js', import.meta.url), 'utf8')
await writeFile(modulePath, source)

const { userErrorMessage } = await import(`file://${modulePath}`)

assert.equal(userErrorMessage(new Error('网络连接异常，请稍后重试'), '提交失败，请重试'), '网络连接异常，请稍后重试')
assert.equal(userErrorMessage({ message: ' 请求超时，请稍后重试 ' }, '提交失败，请重试'), '请求超时，请稍后重试')
assert.equal(userErrorMessage({ errMsg: 'requestPayment:fail cancel' }, '支付失败，请重试'), '已取消支付')
assert.equal(userErrorMessage({ errMsg: 'chooseAvatar:fail cancel' }, '同步失败，请重试'), '已取消操作')
assert.equal(userErrorMessage({}, '提交失败，请重试'), '提交失败，请重试')
assert.equal(userErrorMessage(null, '提交失败，请重试'), '提交失败，请重试')

console.log('user message tests passed')
await rm(dir, { force: true, recursive: true })
