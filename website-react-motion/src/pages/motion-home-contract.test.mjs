import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const root = resolve(new URL('.', import.meta.url).pathname, '..')
const app = readFileSync(resolve(root, 'App.jsx'), 'utf8')
const home = readFileSync(resolve(root, 'pages/Home.jsx'), 'utf8')
const sections = readFileSync(resolve(root, 'pages/homeSections.jsx'), 'utf8')

test('motion site keeps the legacy route surface', () => {
  for (const route of ['teacher', 'stages', 'stage1', 'stage2', 'stage3', 'watch', 'course', 'courses', 'game', 'type/:id', 'quotes', 'mind-quotes', 'mind-quotes/:id', 'types', 'signup']) {
    assert.match(app, new RegExp(`path="${route.replace(/[/:]/g, '\\$&')}"`), `missing legacy route: ${route}`)
  }
})

test('motion home keeps the core business sections and conversion paths', () => {
  for (const marker of ['AppDownloadSection', 'teacher', 'courses', 'home-video', 'stages', 'enterprise', 'QuotesSection', 'TypesSection', 'SignupSection']) {
    assert.match(`${home}\n${sections}`, new RegExp(marker), `missing business marker: ${marker}`)
  }
  assert.match(home, /to="\/game"/, 'game conversion path must remain available')
  assert.match(sections, /submitSignup/, 'signup API integration must remain available')
})

console.log('motion home contract tests passed')
