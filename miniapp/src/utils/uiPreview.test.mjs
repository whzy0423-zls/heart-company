import assert from 'node:assert/strict'
import vm from 'node:vm'
import { readFile } from 'node:fs/promises'
const read = (path) => readFile(new URL(path, import.meta.url), 'utf8')
const source = (await read('./previewTransport.js')).replace(/^import .*\n/gm, '').replace('export async function', 'async function').replaceAll('import.meta.env', 'environment')
const videos = JSON.parse(await read('../data/studioVideos.json'))
const siteConfig = JSON.parse(await read('../data/studioSiteConfig.json'))
function harness(environment) {
  const storage = { nx_token: 'existing-real-session', nx_booking_draft: { contactName: 'real draft' } }
  const context = vm.createContext({ environment, videos, siteConfig, window: { location: { origin: 'http://127.0.0.1:5179' } }, uni: {
    getStorageSync: (key) => storage[key], setStorageSync: (key, value) => { storage[key] = value },
    request: () => { throw new Error('Preview must not use network') },
  } })
  vm.runInContext(source + '\nglobalThis.call = previewRequest', context)
  return { call: context.call, storage }
}
const { call, storage } = harness({ DEV: true, VITE_UI_PREVIEW: 'true' })
const list = await call({ url: '/public/classroom/recent', query: { limit: 2, offset: 1 } })
assert.equal(list.items.length, 2)
assert.equal(list.items[0].id, videos[1].id)
list.items[0].title = 'modified in UI'
assert.notEqual((await call({ url: '/public/classroom/recent', query: { limit: 2, offset: 1 } })).items[0].title, 'modified in UI', 'consumer mutations never overwrite source data')
const booking = await call({ url: '/miniapp/bookings', method: 'POST', data: { kind: 'consult', contactName: '演示学员', phone: '13800000000', intent: '关系沟通' } })
assert.equal(booking.status, 'pending')
assert.equal((await call({ url: '/miniapp/bookings' })).items[0].id, booking.id)
assert.equal(storage.nx_token, 'existing-real-session')
assert.equal(storage.nx_booking_draft.contactName, 'real draft')
assert.ok(storage.nx_ui_preview_bookings)
const playback = await call({ url: `/miniapp/classroom/content/${videos[0].id}/play`, method: 'POST' })
assert.match(playback.url, /^http:\/\/127\.0\.0\.1:5179\/__studio-media\/laohan-\d+\.mp4$/)
await call({ url: `/miniapp/classroom/content/${videos[0].id}/progress`, method: 'PUT', data: { positionSeconds: 12 } })
assert.equal((await call({ url: '/miniapp/classroom/continue-learning' })).items[0].positionSeconds, 12)
await assert.rejects(() => call({ url: '/miniapp/classroom/orders', method: 'POST' }), /尚未纳入演示/, 'unknown writes stop locally')
for (const env of [{ DEV: false, VITE_UI_PREVIEW: 'true' }, { DEV: true }, { DEV: true, VITE_UI_PREVIEW: 'false' }]) {
  await assert.rejects(() => harness(env).call({ url: '/miniapp/bookings', method: 'POST' }), /仅在本地演示/, 'fixtures cannot activate in production or without explicit flag')
}
console.log('UI preview isolation, records and playback tests passed')
