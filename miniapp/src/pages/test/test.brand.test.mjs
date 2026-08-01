import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('./test.vue', import.meta.url), 'utf8')
const template = source.match(/<template>([\s\S]*?)<\/template>/)?.[1] || ''
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1] || ''
const style = source.match(/<style scoped>([\s\S]*?)<\/style>/)?.[1] || ''

assert.ok(template && script && style, 'test page should expose template, script, and scoped style')

for (const text of ['九型测试小游戏', '18 道生活情境题', '约 3 分钟']) {
  assert.match(template, new RegExp(text.replace(/ /g, '\\s*')), `test page should introduce ${text}`)
}
assert.doesNotMatch(template, /gender__card--[mf]/, 'gender choice cards should not use gender-specific theme classes')
assert.doesNotMatch(style, /gender__card--[mf]/, 'styles should not branch into high-saturation gender themes')
for (const token of ['--nx-brand-900', '--nx-brand-700', '--nx-accent-gold', '--nx-page-bg', '--nx-surface', '--nx-text', '--nx-text-muted', '--nx-border']) {
  assert.match(style, new RegExp(`var\\(${token}\\)`), `test page should use brand token ${token}`)
}
assert.doesNotMatch(
  style,
  /#1d4ed8|#4338ca|#6d28d9|#ec4899|#c2410c|#be123c|#047857|#0f766e|#f5f7ff/i,
  'test page should not keep the old saturated blue/purple/gender palette',
)

for (const requiredLogic of [
  /const progress = computed\(/,
  /answerLocked\.value/,
  /function choose\(opt\)/,
  /function back\(\)/,
  /function finish\(\)/,
  /calcType\(answers\.value, gender\.value\)/,
  /reportGameResultApi\(/,
]) {
  assert.match(script, requiredLogic, `test page should preserve quiz logic: ${requiredLogic}`)
}
assert.match(template, /:disabled="answerLocked"/, 'answer locking should still disable options while advancing')
assert.match(template, /v-if="step > 0"[\s\S]{0,180}@click="back"/, 'quiz page should preserve previous-question navigation')
assert.match(style, /@media \(max-width: 360px\)[\s\S]*\.gender__row[\s\S]*flex-direction:\s*column/, 'gender selection should stack on narrow screens')
assert.match(style, /\.gender__card\s*\{[\s\S]*min-width:\s*0/, 'gender cards should be allowed to shrink without horizontal overflow')

console.log('test page brand tests passed')
