import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const footerSource = readFileSync(resolve(__dirname, 'Footer.jsx'), 'utf8')
const configSource = readFileSync(resolve(__dirname, '../../../shared/site-config.json'), 'utf8')
const htmlSource = readFileSync(resolve(__dirname, '../../index.html'), 'utf8')

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

assert.match(
  footerSource,
  /ICP_FILING_URL\s*=\s*['"]https:\/\/beian\.miit\.gov\.cn\/#\/Integrated\/recordQuery['"]/,
  '页脚备案号需要使用工信部备案查询入口常量',
)

assert.match(
  footerSource,
  /href=\{ICP_FILING_URL\}/,
  '页脚备案号链接需要引用工信部备案查询入口常量',
)

assert.match(
  footerSource,
  /鲁ICP备2026051312号-1/,
  '页脚需要展示完整的网站备案号',
)

assert.match(
  htmlSource,
  /<div id=["']root["']>[\s\S]*鲁ICP备2026051312号-1[\s\S]*<\/div>/,
  '首页原始 HTML 需要包含备案号，便于不执行 JavaScript 的审核爬虫查询',
)

assert.match(
  htmlSource,
  /href=[\"']https:\/\/beian\.miit\.gov\.cn\/#\/Integrated\/recordQuery[\"']/,
  '首页原始 HTML 中的备案号需要直达工信部备案查询入口',
)

console.log('footer copyright tests passed')
