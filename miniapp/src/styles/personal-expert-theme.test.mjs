import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const stylesheetPath = fileURLToPath(new URL('./apple-mobile.css', import.meta.url))
const stylesheet = readFileSync(stylesheetPath, 'utf8')

const tokenValue = (name) => {
  const match = stylesheet.match(new RegExp(`${name}\\s*:\\s*([^;]+);`))
  return match?.[1].trim()
}

for (const token of [
  '--nx-brand-900',
  '--nx-brand-700',
  '--nx-accent-gold',
  '--nx-page-bg',
  '--nx-surface',
  '--nx-text',
  '--nx-text-muted',
  '--nx-border',
  '--nx-danger',
  '--nx-success',
]) {
  assert.ok(tokenValue(token), `expected ${token} to be defined`)
}

assert.equal(tokenValue('--nx-brand-900'), '#202A37')
assert.equal(tokenValue('--nx-accent-gold'), '#DFBC7F')
assert.doesNotMatch(stylesheet, /--nx-(?:teal|green)\s*:/)

for (const className of [
  '.nx-page-shell',
  '.nx-card',
  '.nx-button--primary',
  '.nx-button--secondary',
  '.nx-section-head',
]) {
  assert.match(stylesheet, new RegExp(className.replace('.', '\\.')), `expected ${className} contract`)
}

const baseButtonRule = stylesheet.match(/\.nx-button\s*\{([\s\S]*?)\}/)
assert.ok(baseButtonRule, 'expected .nx-button rule')
assert.match(baseButtonRule[1], /min-height\s*:\s*88rpx\s*;/)

assert.equal((stylesheet.match(/:root\s*\{/g) ?? []).length, 1, 'expected a single :root token block')

for (const legacyClass of ['.wrap', '.card', '.btn-primary']) {
  assert.match(stylesheet, new RegExp(legacyClass.replace('.', '\\.')), `expected legacy ${legacyClass} compatibility`)
}

const ruleBody = (selector) => {
  const escaped = selector.replace('.', '\\.')
  const matches = [...stylesheet.matchAll(new RegExp(`^${escaped}\\s*\\{([\\s\\S]*?)\\}`, 'gm'))]
  const match = matches.at(-1)
  assert.ok(match, `expected ${selector} rule`)
  return match[1]
}

assert.match(ruleBody('.btn-primary'), /var\(--nx-brand-(?:900|700)\)/)
assert.match(ruleBody('.btn-ghost'), /var\(--nx-surface\)/)
assert.match(ruleBody('.btn-soft'), /var\(--nx-accent-gold\)/)

assert.match(tokenValue('--nx-page-bottom'), /env\(safe-area-inset-bottom\)/)
assert.match(tokenValue('--nx-page-bottom'), /var\(--window-bottom, 0px\)/)
assert.match(ruleBody('.ios-page'), /padding-bottom\s*:\s*0\s*;/)
assert.doesNotMatch(ruleBody('.page-stack'), /padding-bottom\s*:/)
assert.match(ruleBody('.ios-safe-bottom'), /padding-bottom\s*:\s*var\(--nx-page-bottom\)\s*;/)
assert.doesNotMatch(stylesheet, /constant\(safe-area-inset-bottom\)/)
assert.match(stylesheet, /@media\s*\(prefers-reduced-motion:\s*reduce\)/)

assert.deepEqual(
  Object.fromEntries([
    '--nx-brand-900',
    '--nx-brand-700',
    '--nx-accent-gold',
    '--nx-page-bg',
    '--nx-surface',
    '--nx-text',
    '--nx-text-muted',
    '--nx-border',
    '--nx-danger',
    '--nx-success',
  ].map((token) => [token, tokenValue(token)])),
  {
    '--nx-brand-900': '#202A37',
    '--nx-brand-700': '#314052',
    '--nx-accent-gold': '#DFBC7F',
    '--nx-page-bg': '#F5F3ED',
    '--nx-surface': '#FFFFFF',
    '--nx-text': '#1D2733',
    '--nx-text-muted': '#64748B',
    '--nx-border': '#E0DDD4',
    '--nx-danger': '#B42318',
    '--nx-success': '#157A55',
  },
)

for (const className of ['.nx-button', '.nx-button--text', '.nx-eyebrow', '.nx-title', '.nx-body']) {
  assert.match(stylesheet, new RegExp(className.replace('.', '\\.')), `expected ${className} contract`)
}
assert.match(ruleBody('.nx-button'), /min-height\s*:\s*88rpx\s*;/)
assert.match(ruleBody('.nx-button'), /display\s*:\s*flex\s*;/)
assert.doesNotMatch(ruleBody('.nx-button--primary'), /min-height\s*:/)
assert.doesNotMatch(ruleBody('.nx-button--secondary'), /min-height\s*:/)
assert.match(ruleBody('.nx-button--text'), /min-height\s*:\s*88rpx\s*;/)

const cascadePaddingBottom = (classes) => {
  const matches = [...stylesheet.matchAll(/([^{}]+)\{([^{}]*)\}/g)]
  const candidates = []
  for (const [index, match] of matches.entries()) {
    const padding = match[2].match(/padding-bottom\s*:\s*([^;]+);/)
    if (!padding) continue
    for (const selector of match[1].split(',')) {
      const selectorClasses = [...selector.matchAll(/\.([\w-]+)/g)].map((item) => item[1])
      if (selectorClasses.length && selectorClasses.every((className) => classes.includes(className))) {
        candidates.push({ specificity: selectorClasses.length, index, value: padding[1].trim() })
      }
    }
  }
  candidates.sort((a, b) => a.specificity - b.specificity || a.index - b.index)
  return candidates.at(-1)?.value
}

assert.equal(
  cascadePaddingBottom(['wrap', 'page-stack', 'ios-page', 'ios-safe-bottom']),
  'var(--nx-page-bottom)',
  'common page root keeps its safe-area bottom padding after the CSS cascade',
)

console.log('personal expert theme contract: PASS')
