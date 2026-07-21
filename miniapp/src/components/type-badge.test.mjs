import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('./type-badge.vue', import.meta.url), 'utf8')

for (const prop of ['typeId', 'size', 'selected', 'label', 'disabled']) {
  assert.match(source, new RegExp(`\\b${prop}\\s*:`), `type badge should declare the ${prop} prop`)
}

assert.match(source, /size:\s*\{[\s\S]*default:\s*['"]md['"]/, 'type badge should default to the medium size')
assert.match(source, /selected:\s*\{[\s\S]*default:\s*false/, 'type badge should default to unselected')
assert.match(source, /label:\s*\{[\s\S]*default:\s*['"]['"]/, 'type badge label should be optional')
assert.match(source, /disabled:\s*\{[\s\S]*default:\s*false/, 'type badge should default to enabled')

assert.match(source, /import\s*\{\s*typeTheme\s*\}.*typeTheme/, 'type badge should use the shared type theme utility')
assert.match(source, /typeTheme\(props\.typeId\)/, 'type badge should use the safe theme fallback for its type id')
assert.match(source, /--type-accent/, 'type badge should expose the accent through a CSS variable')
assert.match(source, /--type-soft/, 'type badge should expose the soft color through a CSS variable')
assert.match(source, /--type-ink/, 'type badge should expose the ink color through a CSS variable')

assert.match(source, /type-badge--selected/, 'type badge should expose a selected state class')
assert.match(source, /type-badge--disabled/, 'type badge should expose a disabled state class')
assert.match(source, /aria-disabled/, 'type badge should expose disabled semantics')
assert.match(source, /\{\{\s*typeId\s*\}\}/, 'type badge should render its type number')
assert.match(source, /v-if=["']label["']/, 'type badge should only render label copy when provided')
assert.match(source, /\{\{\s*label\s*\}\}/, 'type badge should render the provided label')
assert.doesNotMatch(source, /type-(?:1|2|3|4|5|6|7|8|9)\b/, 'type badge should not duplicate nine per-type CSS classes')

console.log('type badge source contract tests passed')
