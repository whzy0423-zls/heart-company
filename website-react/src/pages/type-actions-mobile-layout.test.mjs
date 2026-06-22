import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const cssSource = readFileSync(resolve(__dirname, '../index.css'), 'utf8')

assert.match(
  cssSource,
  /@media\s*\(max-width:\s*920px\)\s*\{[\s\S]*\.type-actions\s+\{\s*[\s\S]*flex-direction:\s*column;[\s\S]*\}/,
  '移动端 type-actions 需要纵向布局',
)

assert.match(
  cssSource,
  /\.type-actions\s+\.btn-row\s*\{[^}]*width:\s*100%;/s,
  'type-actions 内的按钮组需要撑满卡片宽度',
)

assert.match(
  cssSource,
  /\.type-actions\s+\.btn-row\s+\.btn\s*\{[^}]*width:\s*100%;/s,
  'type-actions 内每个按钮需要撑满按钮组宽度',
)

console.log('type actions mobile layout tests passed')
