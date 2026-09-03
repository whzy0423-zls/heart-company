import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const navDataSource = readFileSync(resolve(__dirname, 'navData.js'), 'utf8')
const navSource = readFileSync(resolve(__dirname, 'Nav.jsx'), 'utf8')

assert.match(
  navDataSource,
  /export function isActive\(item,\s*pathname,\s*hash\s*=\s*''\)/,
  '顶部导航选中逻辑需要接收 hash，避免 /#courses 等锚点仍选中首页',
)

assert.match(
  navDataSource,
  /const current\s*=\s*pathname\s*\+\s*hash/,
  '导航选中逻辑需要用 pathname + hash 判断锚点导航',
)

assert.match(
  navDataSource,
  /if\s*\(item\.to\s*===\s*'\/'\)\s*return pathname\s*===\s*'\/'\s*&&\s*!hash/,
  '首页只有在无 hash 的根路径才应该选中',
)

assert.match(
  navDataSource,
  /item\.type\s*===\s*'hash'[\s\S]*return current\s*===\s*item\.to/,
  'hash 类型导航需要按完整路径和 hash 选中',
)

assert.match(
  navDataSource,
  /item\.to\s*===\s*'\/courses'[\s\S]*current\s*===\s*'\/#courses'/,
  '课程导航需要兼容旧的 /#courses 画框点击选中',
)

assert.match(
  navSource,
  /const\s*\{\s*pathname,\s*hash\s*\}\s*=\s*useLocation\(\)/,
  '顶部 Nav 需要读取 hash',
)

assert.match(
  navSource,
  /isActive\(it,\s*pathname,\s*hash\)/,
  '顶部 Nav 需要把 hash 传给选中逻辑',
)

console.log('nav active tests passed')
