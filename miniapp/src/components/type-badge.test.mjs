import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { copyFile, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { compileScript, compileStyle, compileTemplate, parse } from '@vue/compiler-sfc'

const source = readFileSync(new URL('./type-badge.vue', import.meta.url), 'utf8')
const filename = 'type-badge.vue'
const { descriptor, errors: parseErrors } = parse(source, { filename })
assert.deepEqual(parseErrors, [], 'type badge should be a valid Vue SFC')
assert.ok(descriptor.scriptSetup, 'type badge should keep a script setup block')
assert.ok(descriptor.template, 'type badge should keep a template block')
compileScript(descriptor, { id: 'type-badge-test' })
const compiledTemplate = compileTemplate({
  source: descriptor.template.content,
  filename,
  id: 'type-badge-test',
})
assert.deepEqual(compiledTemplate.errors, [], 'type badge template should compile')
for (const style of descriptor.styles) {
  const compiledStyle = compileStyle({
    source: style.content,
    filename,
    id: 'type-badge-test',
    scoped: style.scoped,
  })
  assert.deepEqual(compiledStyle.errors, [], 'type badge styles should compile')
}

for (const prop of ['typeId', 'size', 'selected', 'label', 'disabled', 'interactive']) {
  assert.match(source, new RegExp(`\\b${prop}\\s*:`), `type badge should declare the ${prop} prop`)
}

assert.match(source, /size:\s*\{[\s\S]*default:\s*['"]md['"]/, 'type badge should default to the medium size')
assert.match(source, /selected:\s*\{[\s\S]*default:\s*false/, 'type badge should default to unselected')
assert.match(source, /label:\s*\{[\s\S]*default:\s*['"]['"]/, 'type badge label should be optional')
assert.match(source, /disabled:\s*\{[\s\S]*default:\s*false/, 'type badge should default to enabled')
assert.match(source, /interactive:\s*\{[\s\S]*default:\s*false/, 'type badge should default to display-only semantics')

assert.match(source, /import\s*\{\s*typeTheme\s*\}.*typeTheme/, 'type badge should use the shared type theme utility')
assert.match(source, /typeTheme\(props\.typeId\)/, 'type badge should use the safe theme fallback for its type id')
assert.match(source, /--type-accent/, 'type badge should expose the accent through a CSS variable')
assert.match(source, /--type-soft/, 'type badge should expose the soft color through a CSS variable')
assert.match(source, /--type-ink/, 'type badge should expose the ink color through a CSS variable')

assert.match(source, /type-badge--selected/, 'type badge should expose a selected state class')
assert.match(source, /type-badge--disabled/, 'type badge should expose a disabled state class')
assert.match(source, /aria-disabled/, 'type badge should expose disabled semantics')
assert.match(source, /defineEmits\(\[['"]click['"]\]\)/, 'type badge should declare its click event')
assert.match(source, /handleTypeBadgeClick\(props\.interactive,\s*props\.disabled,\s*emit,\s*event\)/, 'type badge should route root taps through the interaction guard')
assert.match(source, /@click=["']onClick["']/, 'type badge root should use the guarded click handler')
assert.match(source, /:role=["']interactive\s*\?\s*['"]button['"]\s*:\s*undefined["']/, 'display badges should only expose button role when interactive')
assert.match(source, /:aria-label=["']interactive\s*\?\s*accessibleLabel\s*:\s*undefined["']/, 'interactive badges should expose a computed accessible label')
assert.match(source, /:aria-pressed=["']interactive\s*\?/, 'selected semantics should only be exposed for interactive badges')
assert.match(source, /:tabindex=["']interactive\s*&&\s*!disabled\s*\?\s*0\s*:\s*undefined["']/, 'enabled interactive badges should be keyboard focusable')
assert.match(source, /@keydown\.enter\.prevent=["']onClick["']/, 'interactive badges should support Enter without browser default side effects')
assert.match(source, /@keydown\.space\.prevent=["']onClick["']/, 'interactive badges should support Space without scrolling the page')
assert.match(source, /type-badge-hit--interactive/, 'interactive badges should expose a separate outer hit target')
assert.match(source, /\.type-badge-hit--interactive\s*\{[\s\S]*min-height:\s*88rpx/, 'interactive badge hit targets should be at least 88rpx tall')
assert.match(source, /class=["']type-badge["']/, 'the compact visual badge should be nested inside the hit target')
assert.match(source, /\{\{\s*typeId\s*\}\}/, 'type badge should render its type number')
assert.match(source, /v-if=["']label["']/, 'type badge should only render label copy when provided')
assert.match(source, /\{\{\s*label\s*\}\}/, 'type badge should render the provided label')
assert.match(source, /\.type-badge__number\s*\{[\s\S]*color:\s*var\(--badge-ink\)/, 'badge digits should use the guaranteed high-contrast ink token')
assert.match(source, /\.type-badge__label\s*\{[\s\S]*color:\s*var\(--badge-ink\)/, 'badge labels should use the guaranteed high-contrast ink token')
assert.doesNotMatch(source, /\.type-badge__number\s*\{[\s\S]*?color:\s*var\(--type-accent\)/, 'accent colors should not be used for small digit text')

const baseStyle = source.match(/(?:^|\n)\.type-badge\s*\{([\s\S]*?)\n\}/)?.[1] || ''
const selectedStyle = source.match(/\.type-badge--selected\s*\{([\s\S]*?)\n\}/)?.[1] || ''
assert.match(baseStyle, /border:\s*2rpx solid var\(--nx-line/, 'unselected badges should use a neutral low-emphasis border')
assert.match(baseStyle, /background:\s*var\(--nx-surface/, 'unselected badges should use the neutral surface background')
assert.match(selectedStyle, /background:\s*var\(--type-soft\)/, 'selected badges should use a stronger type-colored fill')
assert.match(selectedStyle, /box-shadow:\s*inset 0 0 0 4rpx var\(--type-accent\)/, 'selected badges should add a strong accent indicator without changing layout geometry')
assert.doesNotMatch(selectedStyle, /border-width:\s*(?!2rpx)/, 'selected styling should not change border width and cause layout shift')
assert.doesNotMatch(source, /type-(?:1|2|3|4|5|6|7|8|9)\b/, 'type badge should not duplicate nine per-type CSS classes')

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-type-badge-interaction-'))
const modulePath = join(dir, 'typeBadgeInteraction.mjs')
await copyFile(new URL('./typeBadgeInteraction.js', import.meta.url), modulePath)
const { handleTypeBadgeClick } = await import(`file://${modulePath}`)

let emitted = 0
let stopped = 0
let prevented = 0
const event = {
  stopPropagation() { stopped += 1 },
  preventDefault() { prevented += 1 },
}
const emit = (name, value) => {
  assert.equal(name, 'click')
  assert.equal(value, event)
  emitted += 1
}

assert.equal(handleTypeBadgeClick(true, true, emit, event), false)
assert.equal(emitted, 0, 'disabled badges must not emit click')
assert.equal(stopped, 1, 'disabled badges should stop the root tap')
assert.equal(prevented, 1, 'disabled badges should prevent the root tap default')

assert.equal(handleTypeBadgeClick(false, false, emit, event), false)
assert.equal(emitted, 0, 'display-only badges must not emit click')
assert.equal(stopped, 1, 'display-only badges should not swallow parent interactions')
assert.equal(prevented, 1, 'display-only badges should not prevent parent interactions')

assert.equal(handleTypeBadgeClick(true, false, emit, event), true)
assert.equal(emitted, 1, 'enabled badges should emit click')

console.log('type badge source contract tests passed')
await rm(dir, { force: true, recursive: true })
