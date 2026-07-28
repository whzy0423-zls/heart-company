import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('./NxAsyncState.vue', import.meta.url), 'utf8')

assert.match(source, /defineProps\s*\(\s*\{[\s\S]*?state\s*:\s*\{[\s\S]*?required\s*:\s*true[\s\S]*?validator\s*:/, 'state should be required and validated')
for (const state of ['loading', 'stale', 'empty', 'error']) {
  assert.match(source, new RegExp(`['\"]${state}['\"]`), `state validator should allow ${state}`)
  assert.match(source, new RegExp(`nx-async-state--${state}`), `should provide ${state} class`)
}
for (const prop of ['title', 'description', 'actionText', 'busy']) {
  assert.match(source, new RegExp(`${prop}\\s*:`), `should define ${prop} prop`)
}
assert.match(source, /defineEmits\s*\(\s*\[\s*['\"]action['\"]\s*\]\s*\)/, 'should emit action')
assert.match(source, /v-if="actionText"/, 'action should only render with actionText')
assert.match(source, /@click="handleAction"/, 'action button should invoke click guard')
assert.match(source, /:disabled="busy"/, 'busy action should be disabled')
assert.match(source, /处理中…/, 'busy action should announce processing')
assert.match(source, /if\s*\(\s*props\.busy\s*\)\s*return/, 'busy guard should prevent duplicate emits')
assert.match(source, /class="nx-button\s+nx-button--secondary/, 'action should reuse secondary button styles')
assert.match(source, /aria-live="polite"/, 'state updates should be announced politely')
assert.match(source, /role="status"/, 'state container should have status semantics')
assert.match(source, /\.nx-async-state__action\s*\{[\s\S]*?min-height\s*:\s*88rpx\s*;/, 'component action should retain an 88rpx touch target')
assert.match(source, /\.nx-async-state__spinner\s*\{[\s\S]*?animation\s*:/, 'loading should use a CSS spinner')

console.log('NxAsyncState contract tests passed')
