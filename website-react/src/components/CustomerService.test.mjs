import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const __dirname = dirname(fileURLToPath(import.meta.url))
const cssSource = readFileSync(resolve(__dirname, '../index.css'), 'utf8')

test('keeps the mobile customer-service action below the homepage download CTA', () => {
  const mobileRule = cssSource.match(
    /@media\s*\(max-width:\s*760px\)\s*\{[\s\S]*?\.customer-service-fab\s*\{([\s\S]*?)\n\s*\}/,
  )?.[1]

  assert.ok(mobileRule, '需要提供移动端客服悬浮按钮规则')
  assert.match(
    mobileRule,
    /bottom:\s*calc\(80px\s*\+\s*env\(safe-area-inset-bottom\)\)/,
    '客服按钮应位于底部导航上方的安全间距内，避免覆盖首页下载按钮',
  )
})
