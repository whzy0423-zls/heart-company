import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const source = await readFile(new URL('./classroom.vue', import.meta.url), 'utf8').catch(() => '')

assert.ok(source, 'classroom list page should exist')
assert.match(source, /listClassroomSeriesApi/, 'classroom list should load published series')
assert.match(source, /listClassroomStandaloneApi/, 'classroom list should load standalone courseware')
assert.match(source, /getClassroomSeriesApi/, 'classroom list should expand a series into its lessons')
assert.match(source, /normalizeClassroomSeries/, 'series metadata should be normalized before rendering')
assert.match(source, /normalizeClassroomContent/, 'courseware metadata should be normalized before rendering')
assert.match(source, /class="classroom-tabs"[^>]*role="tablist"/, 'classroom should expose a two-entry tab list')
assert.match(source, /activeTab === 'series'/, 'classroom should expose the series entry')
assert.match(source, /activeTab === 'standalone'/, 'classroom should expose the standalone entry')
assert.match(source, />系列课程</, 'series tab should have a clear label')
assert.match(source, />独立课件</, 'standalone tab should have a clear label')
assert.match(source, /v-if="loading"[^>]*class="classroom-state/, 'classroom should render a safe loading state')
assert.match(source, /v-else-if="loadError"[^>]*class="classroom-state classroom-state--error/, 'classroom should render a safe error state')
assert.match(source, /@click="retryActiveList"/, 'list errors should provide retry')
assert.match(source, /v-else-if="activeItems\.length === 0"[^>]*class="classroom-state/, 'classroom should render an empty state')
assert.match(source, /aria-live="polite"/, 'async classroom feedback should be announced politely')
assert.match(source, /function\s+openSeries\s*\(/, 'series cards should open their lesson list')
assert.match(source, /selectedSeries\.value\s*=\s*item/, 'series retry should retain the series the user selected')
assert.match(source, /@click="openSeries\(selectedSeries\)"/, 'series load retry should retry the selected series')
assert.match(source, /if\s*\(!force\s*&&\s*loadedTabs\.value\[tab\]\)\s*\{[\s\S]*?loading\.value\s*=\s*false/, 'switching back to a loaded tab should settle an older loading state')
assert.match(source, /function\s+openContent\s*\(/, 'courseware cards should open content detail')
assert.match(source, /classroomContentRoute/, 'content navigation should use the safe route helper')
assert.match(source, /classroomAccessLabel/, 'list cards should explain effective access')
assert.match(source, /classroomPurchaseAction/, 'list cards should expose the effective access action')
for (const forbidden of [/objectKey/i, /mediaUrl/i, /aliyuncs\.com/i, /oss-[a-z0-9-]+\./i]) {
  assert.doesNotMatch(source, forbidden, 'classroom list source must not read or render permanent media locations')
}

const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1]
assert.ok(script, 'classroom list should expose executable page state')
const executableScript = script.replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*$/gm, '')
const dir = await mkdtemp(join(tmpdir(), 'nx-classroom-page-state-'))
const modulePath = join(dir, 'classroom-state.mjs')
const prelude = `
const ref = (value) => ({ value })
const computed = (getter) => ({ get value() { return getter() } })
const onLoad = (handler) => { globalThis.__classroomHarness.onLoad = handler }
const listClassroomSeriesApi = (...args) => globalThis.__classroomHarness.listSeries(...args)
const listClassroomStandaloneApi = (...args) => globalThis.__classroomHarness.listStandalone(...args)
const getClassroomSeriesApi = (...args) => globalThis.__classroomHarness.getSeries(...args)
const normalizeClassroomSeries = (value = {}) => ({ ...value, id: String(value.id || '') })
const normalizeClassroomContent = (value = {}) => ({ ...value, id: String(value.id || '') })
const classroomAccessLabel = (value) => value
const classroomContentRoute = (item) => item?.id ? '/detail/' + item.id : ''
const classroomPurchaseAction = () => ({ type: 'play', label: '立即学习' })
const userErrorMessage = (error, fallback) => error?.message || fallback
`
await writeFile(modulePath, `${prelude}\n${executableScript}\nexport { activeTab, expandedSeries, selectedSeries, seriesLoading, seriesError, selectTab, openSeries }\n`)

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

let harnessCounter = 0
async function createHarness() {
  const state = {
    listSeries: async () => ({ items: [] }),
    listStandalone: async () => ({ items: [] }),
    getSeries: async () => ({ series: { id: 1 }, contents: [] }),
  }
  globalThis.__classroomHarness = state
  globalThis.uni = { navigateTo() {} }
  harnessCounter += 1
  const page = await import(`${pathToFileURL(modulePath).href}?case=${harnessCounter}`)
  return { page, state }
}

try {
  for (const outcome of ['resolve', 'reject']) {
    const { page, state } = await createHarness()
    const staleSeries = deferred()
    state.getSeries = () => staleSeries.promise
    const oldRequest = page.openSeries({ id: '12', title: '旧系列' })
    assert.equal(page.seriesLoading.value, true)

    page.selectTab('standalone')
    await Promise.resolve()
    assert.equal(page.activeTab.value, 'standalone')
    assert.equal(page.seriesLoading.value, false, 'tab switch should settle the abandoned series loading state')
    assert.equal(page.selectedSeries.value, null, 'tab switch should clear the abandoned series selection')
    assert.equal(page.seriesError.value, '', 'tab switch should clear series feedback')

    if (outcome === 'resolve') staleSeries.resolve({ series: { id: 12, title: '迟到系列' }, contents: [{ id: 21 }] })
    else staleSeries.reject(new Error('迟到失败'))
    await oldRequest
    assert.equal(page.expandedSeries.value, null, `stale series ${outcome} must not write into standalone state`)
    assert.equal(page.seriesError.value, '', `stale series ${outcome} must not write feedback into standalone state`)

    let reloads = 0
    state.listSeries = async () => { reloads += 1; return { items: [] } }
    page.selectTab('series')
    await Promise.resolve()
    await Promise.resolve()
    assert.equal(reloads, 1, 'switching back to series should load its current list normally')
  }
} finally {
  await rm(dir, { force: true, recursive: true })
}

console.log('miniapp classroom list page contract tests passed')
