import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('./relation.vue', import.meta.url), 'utf8')

for (const token of [
  '--nx-brand-900',
  '--nx-brand-700',
  '--nx-accent-gold',
  '--nx-page-bg',
  '--nx-surface',
  '--nx-surface-soft',
  '--nx-text',
  '--nx-text-muted',
  '--nx-border',
]) {
  assert.ok(source.includes(`var(${token})`), `relation page should use ${token}`)
}

assert.match(
  source,
  /<view class="wrap relation page-stack ios-page ios-safe-bottom">/,
  'relation page should use the shared page shell',
)
assert.doesNotMatch(
  source,
  /#(?:6d28d9|db2777|4c1d95|7e22ce|be185d|9333ea|86198f|2dd4bf)/i,
  'relation page should not switch to the old saturated purple, pink, or green palette',
)
assert.doesNotMatch(
  source,
  /<view class="wrap relation[^>]*\s:class=|relation--\$\{?(?:myType|taType|center)/,
  'personality differences should stay inside local labels and graphics instead of changing the page theme',
)
assert.match(source, /\.type-chip\s*\{[\s\S]*?min-height:\s*128rpx/, 'type choices should keep generous touch targets')
assert.match(source, /\.btn-primary,\s*\.btn-ghost\s*\{[\s\S]*?min-height:\s*88rpx/, 'relation actions should be at least 88rpx high')
assert.match(source, /v-if="stage === 'pick'"/, 'relation type picker state should remain available')
assert.match(source, /v-else-if="stage === 'result'"/, 'relation result state should remain available')
assert.match(source, /@click="analyze"/, 'relation analysis action should remain available')
assert.match(source, /@click="reset"/, 'relation reset action should remain available')
assert.match(source, /onUnload\([\s\S]*clearTimeout\(redirectTimer\)/, 'invalid route redirects should be cancelled when the page unloads')

console.log('relation brand tests passed')
