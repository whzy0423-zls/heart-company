import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('./NxAsyncState.vue', import.meta.url), 'utf8')

assert.match(source, /state\s*:\s*\{[\s\S]*?required\s*:\s*true/, 'state should be required')

const validatorMatch = source.match(/validator:\s*\(value\)\s*=>\s*\[([^\]]+)\]\.includes\(value\)/)
assert.ok(validatorMatch, 'state should define an allowed-values validator')
const allowedStates = [...validatorMatch[1].matchAll(/['"]([^'"]+)['"]/g)].map((match) => match[1]).sort()
assert.deepEqual(allowedStates, ['empty', 'error', 'loading', 'stale'], 'state validator should allow exactly the four unified states')

for (const state of allowedStates) {
  assert.match(source, new RegExp(`nx-async-state--${state}`), `should provide ${state} class`)
}
for (const prop of ['title', 'description', 'actionText', 'busy']) {
  assert.match(source, new RegExp(`${prop}\\s*:`), `should define ${prop} prop`)
}
assert.match(source, /defineEmits\s*\(\s*\[\s*['"]action['"]\s*\]\s*\)/, 'should emit action')
assert.match(source, /v-if="actionText"/, 'action should only render with actionText')
assert.match(source, /@click="handleAction"/, 'action button should invoke click guard')
assert.match(source, /:disabled="busy"/, 'busy action should be disabled')
assert.match(source, /处理中…/, 'busy action should announce processing')

const guardMatch = source.match(/function handleAction\(\)\s*\{([\s\S]*?)\n\}/)
assert.ok(guardMatch, 'should define an action handler')
const guardBody = guardMatch[1]
const guardIndex = guardBody.search(/if\s*\(\s*props\.busy\s*\|\|\s*!props\.actionText\s*\)\s*return/)
const emitIndex = guardBody.search(/emit\(['"]action['"]\)/)
assert.ok(guardIndex >= 0, 'busy or missing actionText should guard against emits')
assert.ok(emitIndex > guardIndex, 'action should only emit after the busy/actionText guard')

assert.match(source, /class="nx-button\s+nx-button--secondary/, 'action should reuse secondary button styles')
assert.match(source, /aria-live="polite"/, 'state updates should be announced politely')
assert.match(source, /role="status"/, 'state container should have status semantics')
assert.doesNotMatch(source, /:aria-busy=/, 'live state region should not remain aria-busy')
assert.match(source, /\.nx-async-state__action\s*\{[\s\S]*?min-height\s*:\s*88rpx\s*;/, 'component action should retain an 88rpx touch target')
assert.match(source, /\.nx-async-state__spinner\s*\{[\s\S]*?animation\s*:/, 'loading should use a CSS spinner')
assert.match(source, /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{[\s\S]*?\.nx-async-state__spinner\s*\{[\s\S]*?animation\s*:\s*none\s*;/, 'spinner should respect reduced-motion preferences')

console.log('NxAsyncState contract tests passed')
