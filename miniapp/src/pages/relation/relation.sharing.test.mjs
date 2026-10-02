import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { TYPES_INFO } from '../../data/enneagramGame.js'
import { normalizeTypeId, isValidTypeId } from '../../utils/session.js'
import { buildShareCard } from '../../utils/share.js'
import { buildRelationAnalysis } from './relationAnalysis.js'

const source = readFileSync(new URL('./relation.vue', import.meta.url), 'utf8')
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)[1]
  .replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*;?\s*$/gm, '')
function harness(timeline = false) {
  const state = { menus: [], redirects: [], previews: [], hints: 0, timers: new Map() }
  const context = vm.createContext({
    ref: value => ({ value }), nextTick: fn => fn(), TYPES_INFO, normalizeTypeId, isValidTypeId, buildShareCard, buildRelationAnalysis,
    onLoad: fn => { state.load = fn }, onShow: fn => { state.show = fn }, onUnload: fn => { state.unload = fn },
    onShareAppMessage: fn => { state.friend = fn }, onShareTimeline: fn => { state.timeline = fn },
    isTimelinePreview: () => timeline,
    requireFullMiniapp: () => { if (timeline) state.hints++; return !timeline },
    showPublicShareMenu: enabled => state.menus.push(enabled),
    previewImage: (current, options) => state.previews.push({ current, options }),
    setTimeout: fn => { state.timers.set(1, fn); return 1 }, clearTimeout: id => state.timers.delete(id),
    uni: { showToast() {}, pageScrollTo() {}, redirectTo: o => state.redirects.push(o) },
  })
  vm.runInContext(`${script}\nglobalThis.page = { stage, myType, taType, analysis, sharedMode, routeError, analyze, reset, pickMy, pickTa, previewMyAvatar, previewTaAvatar }`, context)
  return { state, page: context.page }
}
for (const timeline of [false, true]) {
  for (let mine = 1; mine <= 9; mine++) for (let ta = 1; ta <= 9; ta++) {
    const { state, page } = harness(timeline)
    state.load({ myType: String(mine), taType: String(ta), token: 'secret', recordId: 'private' })
    state.show()
    assert.equal(page.stage.value, 'result')
    assert.equal(page.sharedMode.value, true)
    assert.equal(page.myType.value, mine)
    assert.equal(page.taType.value, ta)
    assert.deepEqual(page.analysis.value, buildRelationAnalysis(mine, ta))
    assert.equal(state.friend().path, `/pages/relation/relation?myType=${mine}&taType=${ta}`)
    assert.equal(state.timeline().query, `myType=${mine}&taType=${ta}`)
    assert.match(state.friend().imageUrl, /^https:\/\//)
    assert.doesNotMatch(JSON.stringify(state.friend()), /secret|private/)
    assert.equal(state.redirects.length, 0)
  }
}
for (const query of [{ myType: '2' }, { taType: '5' }, { myType: '0', taType: '5' }, { myType: '02', taType: '5' }, { myType: '2', taType: '5&token=x' }, { myType: ['2'], taType: '5' }, { myType: {}, taType: '5' }]) {
  const { state, page } = harness(true)
  state.load(query); state.show()
  assert.equal(page.stage.value, 'pick')
  assert.equal(page.analysis.value, null)
  assert.ok(page.routeError.value)
  assert.equal(state.friend().path, '/pages/relation/relation')
  assert.equal(state.timeline().query, '')
  assert.equal(state.timers.size, 0)
  assert.equal(state.redirects.length, 0)
}
{
  const { state, page } = harness()
  state.load({ type: '2' }); state.show()
  assert.equal(page.myType.value, 2, 'old type-detail entry still prefills the first type')
  assert.equal(page.stage.value, 'pick')
  page.pickTa(5); page.analyze()
  assert.equal(page.stage.value, 'result')
  page.previewMyAvatar(); page.previewTaAvatar()
  assert.equal(state.previews[0].current, '/static/enneagram/2.png')
  assert.equal(state.previews[1].current, '/static/enneagram/5.png')
  assert.equal(state.friend().path, '/pages/relation/relation?myType=2&taType=5')
  page.reset()
  assert.equal(page.stage.value, 'pick')
  assert.equal(page.sharedMode.value, false)
  assert.equal(page.analysis.value, null)
  assert.equal(state.timeline().query, '', 'reset cannot share the previous pair')
  page.pickMy(9); page.pickTa(1); page.analyze()
  assert.equal(state.timeline().query, 'myType=9&taType=1')
}
{
  const { state, page } = harness(true)
  state.load({ myType: '2', taType: '5' }); page.reset()
  assert.equal(page.stage.value, 'result', 'Timeline readers keep their shared result when full app is required')
  assert.equal(state.hints, 1)
}
{
  const { state, page } = harness()
  state.load({ type: 'bad' }); state.show()
  assert.equal(page.stage.value, 'redirecting')
  assert.equal(state.menus.at(-1), false)
  state.unload()
  assert.equal(state.timers.size, 0, 'old invalid-link timer cancels on unload')
}
assert.match(source, /<NxShareActions/)
assert.doesNotMatch(source, /analysis\.score|契合指数/)
console.log('Relation sharing: 81 pairs in friend/Timeline cold starts, invalid recovery, reset and original preview passed')
