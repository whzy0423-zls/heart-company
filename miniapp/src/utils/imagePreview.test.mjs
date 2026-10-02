import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import vm from 'node:vm'
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

// Match the compiler's module-scoped uni binding without a global bridge.
const source = await readFile(new URL('./imagePreview.js', import.meta.url), 'utf8')
const runtimeCalls = []
const runtime = {
  getSystemInfoSync: () => ({ platform: 'ios' }),
  getImageInfo: ({ src, success }) => {
    runtimeCalls.push({ imageSource: src })
    success({ path: 'wxfile://original-teacher.jpg' })
  },
  previewImage: (options) => runtimeCalls.push(options),
}
const sandbox = vm.createContext({ runtime })
vm.runInContext(`
  const uni = runtime;
  ${source.replace(/^export /gm, '')}
  globalThis.previewApi = { previewImage, isWechatDevtools };
`, sandbox)
assert.equal(sandbox.uni, undefined, 'the regression harness must not install a global uni bridge')
assert.equal(sandbox.previewApi.isWechatDevtools(), false, 'the module runtime identifies real devices')
assert.equal(sandbox.previewApi.previewImage('/static/teacher/portrait.jpg'), true, 'a module-scoped runtime must open image preview')
assert.deepEqual(JSON.parse(JSON.stringify(runtimeCalls)), [
  { imageSource: '/static/teacher/portrait.jpg' },
  { current: 'wxfile://original-teacher.jpg', urls: ['wxfile://original-teacher.jpg'] },
], 'bundled original images should resolve to a native preview path')
runtime.getSystemInfoSync = () => ({ platform: 'devtools' })
assert.equal(sandbox.previewApi.isWechatDevtools(), true, 'the module runtime identifies developer tools')

console.log('image preview tests passed')
