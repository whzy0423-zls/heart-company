import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const footerSource = readFileSync(resolve(__dirname, 'Footer.jsx'), 'utf8')
const configSource = readFileSync(resolve(__dirname, '../../../shared/site-config.json'), 'utf8')

assert.doesNotMatch(configSource, /仅作展示用途/, '默认站点配置版权信息不能包含“仅作展示用途”')

assert.match(
  footerSource,
  /replace\(\s*\/\s*\\s\*·\\s\*仅作展示用途\s*\/g,\s*''\s*\)/,
  '页脚需要兜底移除后台旧配置里的“仅作展示用途”',
)

assert.doesNotMatch(
  footerSource,
  /\{siteConfig\.site\.copyright\}/,
  'Footer 不能直接渲染未清理的 copyright',
)

console.log('footer copyright tests passed')
