import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const cssSource = readFileSync(resolve(__dirname, '../index.css'), 'utf8')

assert.match(
  cssSource,
  /\.orbit\s*\{[\s\S]*isolation:\s*isolate;/,
  'orbit 需要建立独立层级，避免内部虚线和 logo 混在同一绘制层',
)

assert.match(
  cssSource,
  /\.orbit::before,\s*\.orbit::after\s*\{[\s\S]*z-index:\s*0;/,
  'orbit 内部虚线需要放到底层，不能盖住中间 logo',
)

assert.match(
  cssSource,
  /\.orbit\s+\.chip-logo\s*\{[\s\S]*position:\s*relative;[\s\S]*z-index:\s*2;/,
  '中间 logo 需要明确高于 orbit 虚线',
)

assert.match(
  cssSource,
  /\.wheel__node\s*\{[\s\S]*z-index:\s*3;/,
  '外圈型号节点需要保持在装饰光环上方',
)

console.log('wheel layering tests passed')
