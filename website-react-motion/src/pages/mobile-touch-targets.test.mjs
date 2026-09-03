import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const cssSource = readFileSync(resolve(__dirname, '../index.css'), 'utf8')

assert.match(
  cssSource,
  /\.type-detail__nav\s+a\s*\{[^}]*min-width:\s*44px;[^}]*min-height:\s*44px;/s,
  '九型详情切换按钮触控热区需要至少 44×44',
)

assert.match(
  cssSource,
  /\.course__dots\s+button\s*\{[^}]*width:\s*44px;[^}]*height:\s*44px;/s,
  '课程点导航按钮触控热区需要至少 44×44',
)

assert.match(
  cssSource,
  /\.course__dots\s+button::after\s*\{[^}]*width:\s*8px;[^}]*height:\s*8px;/s,
  '课程点导航需要保留小圆点视觉，不应把视觉点直接放大到 44px',
)

assert.match(
  cssSource,
  /\.music\s*\{[^}]*bottom:\s*calc\(96px\s*\+\s*env\(safe-area-inset-bottom\)\);/s,
  '移动端音乐控件需要避开底部 tabbar 和安全区',
)

console.log('website mobile touch target tests passed')
