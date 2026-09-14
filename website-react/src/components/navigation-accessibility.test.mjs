import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const readSource = (path) => readFileSync(resolve(__dirname, path), 'utf8')
const drawerSource = readSource('./Drawer.jsx')
const navSource = readSource('./Nav.jsx')
const layoutSource = readSource('./Layout.jsx')
const cssSource = readSource('../index.css')

test('mobile drawer behaves as a keyboard-accessible modal', () => {
  assert.match(drawerSource, /role="dialog"/)
  assert.match(drawerSource, /aria-modal="true"/)
  assert.match(drawerSource, /aria-label="栏目菜单"/)
  assert.match(drawerSource, /aria-label="关闭栏目菜单"/)
  assert.match(drawerSource, /event\.key === 'Escape'/)
  assert.match(drawerSource, /event\.key !== 'Tab'/)
  assert.match(drawerSource, /document\.activeElement/)
  assert.match(drawerSource, /window\.setTimeout\(\(\) => getFocusableElements\(\)\[0\]\?\.focus\(\), 0\)/)
  assert.match(drawerSource, /lastActiveElementRef\.current\?\.focus/)
  assert.match(navSource, /aria-expanded=\{drawerOpen\}/)
  assert.match(navSource, /aria-controls="site-drawer"/)
})

test('route and hash navigation move focus and expose a skip link', () => {
  assert.match(layoutSource, /className="skip-link"/)
  assert.match(layoutSource, /href="#main-content"/)
  assert.match(layoutSource, /<main id="main-content" tabIndex="-1">/)
  assert.match(layoutSource, /target\.focus\(\{ preventScroll: true \}\)/)
  assert.match(layoutSource, /target\.scrollIntoView/)
  assert.match(cssSource, /\.skip-link:focus-visible\s*\{/)
})

test('small text color tokens meet WCAG AA on the page background', () => {
  const token = (name) => {
    const match = cssSource.match(new RegExp(`--${name}:\\s*(#[0-9a-f]{6})`, 'i'))
    assert.ok(match, `缺少 --${name} 色值`)
    return match[1]
  }
  const luminance = (hex) => {
    const channels = hex.slice(1).match(/../g).map((part) => Number.parseInt(part, 16) / 255)
    const linear = channels.map((value) => (
      value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
    ))
    return 0.2126 * linear[0] + 0.7152 * linear[1] + 0.0722 * linear[2]
  }
  const contrast = (foreground, background) => {
    const values = [luminance(foreground), luminance(background)].sort((a, b) => b - a)
    return (values[0] + 0.05) / (values[1] + 0.05)
  }

  for (const name of ['muted', 'blue', 'red']) {
    assert.ok(contrast(token(name), '#f7f8fb') >= 4.5, `--${name} 在页面底色上未达到 4.5:1`)
    assert.ok(contrast(token(name), '#ffffff') >= 4.5, `--${name} 在白色上未达到 4.5:1`)
  }
})
