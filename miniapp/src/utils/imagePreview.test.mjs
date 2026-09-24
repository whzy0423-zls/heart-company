import assert from 'node:assert/strict'
import { previewImage } from './imagePreview.js'

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

console.log('image preview tests passed')
