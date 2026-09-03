import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const homeSectionsSource = readFileSync(resolve(__dirname, 'homeSections.jsx'), 'utf8')
const cssSource = readFileSync(resolve(__dirname, '../index.css'), 'utf8')

assert.match(
  homeSectionsSource,
  /as="blockquote"\s+className="quote-card card"/,
  '语录卡片需要使用 quote-card class，避免新增短语录漏掉双引号装饰',
)

assert.match(
  cssSource,
  /\.quote-card::after\s*\{[^}]*content:\s*"”"/s,
  'quote-card 需要通过 ::after 渲染右侧双引号装饰',
)

assert.match(
  cssSource,
  /\.quote-card\s*\{[^}]*min-height:/s,
  'quote-card 需要保留稳定高度，短语录也能显示完整装饰',
)

console.log('quote card layout tests passed')
