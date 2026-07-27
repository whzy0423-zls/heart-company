import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

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

console.log('miniapp classroom list page contract tests passed')
