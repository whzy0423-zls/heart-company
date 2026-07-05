import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const mainSource = readFileSync(new URL('../src/main.jsx', import.meta.url), 'utf8')
const styleSource = readFileSync(new URL('../src/styles/index.css', import.meta.url), 'utf8')
const readmeSource = readFileSync(new URL('../README.md', import.meta.url), 'utf8')

assert.match(
  mainSource,
  /HashRouter/,
  'reading H5 should use hash routing so /#/article/:id deep links work in WeChat/CDN deployments',
)
assert.match(
  mainSource,
  /migrateLegacyBrowserArticlePath/,
  'reading H5 should migrate old /article/:id browser-router links when the static index is reached',
)
assert.match(
  mainSource,
  /window\.history\.replaceState/,
  'legacy article path migration should replace the URL before HashRouter starts',
)
assert.doesNotMatch(
  mainSource,
  /BrowserRouter/,
  'reading H5 should not require server-side history fallback for article navigation',
)
assert.match(
  readmeSource,
  /\/#\/article\/:id/,
  'deployment docs should describe hash article deep links',
)

for (const [selector, property] of [
  ['.search', 'min-height: 44px'],
  ['.back-btn', 'min-width: 44px'],
  ['.back-btn', 'min-height: 44px'],
  ['.chip', 'min-height: 44px'],
  ['.load-more', 'min-height: 44px'],
  ['.audio-range', 'min-height: 44px'],
  ['.audio-speed', 'min-height: 44px'],
]) {
  const start = styleSource.indexOf(`${selector} {`)
  assert.notEqual(start, -1, `${selector} should exist`)
  const end = styleSource.indexOf('}', start)
  const block = styleSource.slice(start, end)
  assert.match(block, new RegExp(property), `${selector} should include ${property} for iOS touch target compliance`)
}


assert.match(
  styleSource,
  /\.search\s+input\s*\{[^}]*font-size:\s*16px;/s,
  '搜索输入框字体需要至少 16px，避免 iOS 聚焦自动放大页面',
)
assert.match(
  styleSource,
  /\.state-retry\s*\{[^}]*min-height:\s*44px;/s,
  '错误态重试按钮触控热区需要至少 44px',
)

const listSource = readFileSync(new URL('../src/pages/ListPage.jsx', import.meta.url), 'utf8')
const readerSource = readFileSync(new URL('../src/pages/ReaderPage.jsx', import.meta.url), 'utf8')

assert.match(
  listSource,
  /role="alert"[\s\S]*aria-live="assertive"/,
  '列表页错误态需要 role=alert 和 aria-live，便于读屏提示失败',
)
assert.match(
  listSource,
  /className="state-retry"[\s\S]*onClick=\{\(\) => load\(1, true\)\}/,
  '列表页错误态需要提供重试按钮重新加载第一页',
)
assert.match(
  listSource,
  /const\s+requestIdRef\s*=\s*useRef\(0\)/,
  '列表页需要 requestIdRef 防止旧搜索/分类响应覆盖新结果',
)
assert.match(
  listSource,
  /currentRequestId\s*!==\s*requestIdRef\.current/,
  '列表页加载结果落库前需要丢弃 stale response',
)
assert.match(
  listSource,
  /<button[\s\S]*className="card"[\s\S]*onClick=\{\(\) => navigate\(`\/article\/\$\{article\.id\}`\)\}/,
  '文章卡片需要使用 button 语义，不能只用可点击 article',
)
assert.doesNotMatch(
  listSource,
  /<article[\s\S]*onClick=\{\(\) => navigate\(`\/article\/\$\{article\.id\}`\)\}/,
  '文章卡片不能使用不可键盘聚焦的 article onClick',
)

assert.match(
  readerSource,
  /role="alert"[\s\S]*aria-live="assertive"/,
  '详情页错误态需要 role=alert 和 aria-live，便于读屏提示失败',
)
assert.match(
  readerSource,
  /className="state-retry"[\s\S]*onClick=\{loadArticle\}/,
  '详情页错误态需要提供重试按钮重新加载当前文章',
)

console.log('reading H5 routing and touch target tests passed')
