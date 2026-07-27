import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('./classroom-detail.vue', import.meta.url), 'utf8').catch(() => '')

assert.ok(source, 'classroom detail page should exist')
assert.match(source, /getClassroomContentApi/, 'detail should load safe public metadata')
assert.match(source, /normalizeClassroomContent/, 'detail should normalize metadata before rendering')
assert.match(source, /classroomPurchaseAction/, 'detail CTA should follow effective access')
assert.match(source, /classroomAccessLabel/, 'detail should explain effective access')
assert.match(source, /v-if="loading"[^>]*class="detail-state/, 'detail should render a loading state')
assert.match(source, /v-else-if="loadError"[^>]*class="detail-state detail-state--error/, 'detail should render an error state')
assert.match(source, /@click="loadDetail"/, 'detail load errors should provide retry')
assert.match(source, /aria-live="polite"/, 'detail async feedback should be announced politely')
assert.match(source, /<video\b[^>]*v-if="content\.contentType === 'video'"[^>]*:src="playbackUrl"[^>]*@error="handlePlaybackError"/s, 'video courseware should use the page video player and recover playback errors')
assert.match(source, /uni\.createInnerAudioContext\(\)/, 'audio courseware should use a page-scoped audio player')
assert.match(source, /function\s+toggleAudio\s*\(/, 'audio player should support play and pause')
assert.match(source, /function\s+seekAudio\s*\(/, 'audio player should support seeking')
assert.match(source, /<slider\b[^>]*@change="seekAudio"/, 'audio player should expose a seek control')
assert.match(source, /播放音频|暂停音频/, 'audio controls should expose clear play and pause labels')
assert.match(source, /withClassroomPlaybackRetry/, 'playback should use the signed URL retry contract')
assert.match(source, /function\s+refreshPlayback\s*\(/, 'detail should explicitly refresh an expired signed URL')
assert.match(source, /playbackRecoveryUsed/, 'runtime media expiry should allow one automatic signed URL recovery')
assert.match(source, /if\s*\(!playbackRecoveryUsed\)[\s\S]*?refreshPlayback\(\{\s*recovery:\s*true\s*\}\)/, 'the first runtime playback error should refresh its signed URL automatically')
assert.match(source, /@click="refreshPlayback"/, 'playback errors should provide a retry action')
assert.match(source, /playback\?*\.url/, 'only the temporary playback response should provide the player URL')
assert.match(source, /playbackUrl\.value\s*=\s*''/, 'stale signed URLs should be cleared before refresh or teardown')
assert.match(source, /v-if="!content\.canPlay"[^>]*class="access-panel/, 'locked content should render a permission panel instead of a player')
assert.match(source, /@click="handleAccessAction"/, 'permission CTA should have an action')
assert.match(source, /登录后学习|开通会员|购买/, 'permission CTA should cover login, membership, and paid content')
assert.doesNotMatch(source, /createClassroomOrderApi|requestPayment|getClassroomOrderStatusApi/, 'purchase state machine belongs to Task 10')
assert.doesNotMatch(source, /createClassroomProgressTracker|updateClassroomProgressApi|continue-learning/i, 'full progress UX belongs to Task 10')
for (const forbidden of [/objectKey/i, /mediaUrl/i, /aliyuncs\.com/i, /oss-[a-z0-9-]+\./i]) {
  assert.doesNotMatch(source, forbidden, 'detail source must not read or render permanent media locations')
}

console.log('miniapp classroom detail page contract tests passed')
