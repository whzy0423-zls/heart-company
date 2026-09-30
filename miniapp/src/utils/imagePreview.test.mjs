import assert from 'node:assert/strict'
import { isWechatDevtools, previewImage } from './imagePreview.js'

const calls = []
const preview = (options) => calls.push(options)

assert.equal(previewImage('', { preview }), false, 'blank images should not open a preview')
assert.equal(calls.length, 0)

assert.equal(
  previewImage(' /static/avatars/9.png ', { preview, urls: ['/static/avatars/9.png', '/static/avatars/9.png', '/static/avatars/8.png'] }),
  true,
)
assert.deepEqual(calls.at(-1), {
  current: '/static/avatars/9.png',
  urls: ['/static/avatars/9.png', '/static/avatars/8.png'],
})

assert.equal(
  previewImage('https://example.com/teacher.jpg', { preview }),
  true,
)
assert.deepEqual(calls.at(-1), {
  current: 'https://example.com/teacher.jpg',
  urls: ['https://example.com/teacher.jpg'],
})

const previousUni = globalThis.uni
const previousWx = globalThis.wx
globalThis.wx = { previewImage() {} }
globalThis.uni = { getSystemInfoSync: () => ({ platform: 'ios' }) }
assert.equal(isWechatDevtools(), false, 'real-device platforms must not be treated as developer tools')
globalThis.uni = { getSystemInfoSync: () => ({ platform: 'devtools' }) }
assert.equal(isWechatDevtools(), true, 'developer tools platform should be recognized')
if (previousUni === undefined) delete globalThis.uni
else globalThis.uni = previousUni
if (previousWx === undefined) delete globalThis.wx
else globalThis.wx = previousWx

console.log('image preview tests passed')
