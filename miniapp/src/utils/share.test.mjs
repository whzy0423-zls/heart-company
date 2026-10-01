import assert from 'node:assert/strict'
import { buildShareCard, showPublicShareMenu, hidePrivateShareMenu, showTimelineShareHint, isTimelinePreview, requireFullMiniapp } from './share.js'

const home = buildShareCard({ kind: 'home' })
assert.equal(home.appMessage.path, '/pages/index/index')
assert.equal(home.timeline.query, '')
assert.ok(home.appMessage.imageUrl)
for (const [kind, route] of [['teacher','teacher'], ['daily','learn'], ['explore','enneagram']]) {
  const card = buildShareCard({ kind, title: ' 后台更新后的标题 ', token: 'secret', bookingId: 3 })
  assert.equal(card.appMessage.path, `/pages/${route}/${route}`)
  assert.equal(card.appMessage.title, '后台更新后的标题')
  assert.equal(card.timeline.query, '')
}
const course = buildShareCard({ kind: 'course', id: 'course-real_3', title: '关系课程', imageUrl: '/static/editorial/course-classroom.jpg', bookingId: 3, paid: true, token: 'secret' })
assert.equal(course.appMessage.path, '/pages/course-detail/course-detail?id=course-real_3')
assert.equal(course.timeline.query, 'id=course-real_3')
assert.equal(course.appMessage.imageUrl, '/static/editorial/course-classroom.jpg')
assert.equal(JSON.stringify(course).includes('secret'), false)
for (const id of ['', '../orders', 'x&paid=true', '中文', 'a'.repeat(81)]) assert.deepEqual(buildShareCard({ kind: 'course', id }), home)
const media = buildShareCard({ kind: 'content', id: '42', title: '公开短讲', playbackUrl: 'https://media.example/video?token=secret', position: 15 })
assert.equal(media.appMessage.path, '/pages/classroom-detail/classroom-detail?id=42')
assert.equal(media.timeline.query, 'id=42')
assert.equal(JSON.stringify(media).includes('secret'), false)
assert.equal(buildShareCard({ kind: 'content', id: '0' }).appMessage.path, home.appMessage.path)
const series = buildShareCard({ kind: 'classroom', seriesId: '17' })
assert.equal(series.timeline.query, 'tab=series&seriesId=17')
assert.equal(series.appMessage.path, '/pages/classroom/classroom?tab=series&seriesId=17')
assert.equal(buildShareCard({ kind: 'classroom', seriesId: 'x' }).timeline.query, '')
for (const type of [1, 5, 9]) {
  const result = buildShareCard({ kind: 'result', type })
  assert.equal(result.appMessage.path, `/pages/result/result?shareType=${type}`)
  assert.equal(result.timeline.query, `shareType=${type}`)
  assert.equal(result.appMessage.imageUrl, `/static/share/result-${type}.jpg`)
  assert.equal(buildShareCard({ kind: 'type', type }).timeline.query, `type=${type}`)
}
for (const type of [undefined, null, 0, 10, 1.5, '1&token=x']) {
  assert.equal(buildShareCard({ kind: 'result', type }).timeline.query, 'shareType=0')
  assert.equal(buildShareCard({ kind: 'result', type }).appMessage.path, '/pages/result/result?shareType=0')
}
for (const imageUrl of ['https://a.example/cover.jpg?token=secret', 'https://a.example/cover?X-Amz-Signature=secret', 'http://remote.example/cover.jpg', 'wxfile://tmp/photo.jpg', '/static/../private.png', 'javascript:alert(1)', '/static/wheel.svg', '/static/course-intro.webp', 'https://cdn.example/cover.webp']) {
  const card = buildShareCard({ kind: 'home', imageUrl })
  assert.equal(card.appMessage.imageUrl, home.appMessage.imageUrl, imageUrl)
}
assert.equal(buildShareCard({ kind: 'home', imageUrl: 'https://cdn.example/cover.png' }).appMessage.imageUrl, 'https://cdn.example/cover.png')
assert.equal(buildShareCard({ kind: 'teacher', imageUrl: '/static/teacher/portrait.jpg' }).appMessage.imageUrl, '/static/share/studio.jpg', 'built-in tall portrait uses a composed card that retains the face')
const menus = []
const runtime = { showShareMenu: v => menus.push(['show',v]), hideShareMenu: v => menus.push(['hide',v]), showModal: v => menus.push(['hint',v]) }
showPublicShareMenu(true, runtime)
showPublicShareMenu(false, runtime)
hidePrivateShareMenu(runtime)
showTimelineShareHint(runtime)
assert.deepEqual(menus.map(v=>v[0]), ['show','hide','hide','hint'])
assert.deepEqual(menus[0][1].menus, ['shareAppMessage','shareTimeline'])
assert.match(menus[3][1].content, /右上角/)
assert.equal(menus[3][1].showCancel, false)
assert.doesNotThrow(()=>showPublicShareMenu(true, {}))
assert.doesNotThrow(()=>hidePrivateShareMenu({}))
const previewMenus = []
const preview = { getEnterOptionsSync: () => ({ scene: 1154 }), showShareMenu: () => previewMenus.push('show'), hideShareMenu: () => previewMenus.push('hide'), showModal: () => previewMenus.push('modal') }
assert.equal(isTimelinePreview(preview), true)
showPublicShareMenu(true, preview)
hidePrivateShareMenu(preview)
showTimelineShareHint(preview)
assert.deepEqual(previewMenus, [], 'Timeline single-page does not call unsupported native share APIs')
assert.equal(isTimelinePreview({ getEnterOptionsSync: () => ({ scene: 1001 }), getLaunchOptionsSync: () => ({ scene: 1154 }) }), false, 'latest entry mode wins over old launch')
assert.equal(isTimelinePreview({ getEnterOptionsSync: () => { throw Error('old SDK') }, getLaunchOptionsSync: () => ({ scene: 1154 }) }), true)
assert.equal(isTimelinePreview({}), false)
assert.equal(requireFullMiniapp({}), true)
assert.equal(requireFullMiniapp(preview), false)
assert.deepEqual(previewMenus, ['modal'], 'single-page navigation guides the visitor without calling login/payment/routing')
console.log('Public share payload, cold links, privacy and menu tests passed')
