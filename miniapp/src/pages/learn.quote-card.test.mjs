import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const learnSource = readFileSync(resolve(__dirname, 'learn/learn.vue'), 'utf8')

assert.match(
  learnSource,
  /const\s+quotes\s*=\s*ref\(\[\]\)/,
  '学习页需要读取官网语录配置，新增语录才能同步显示',
)

assert.match(
  learnSource,
  /class="quote-card"/,
  '学习页语录卡片需要 quote-card class',
)

assert.match(
  learnSource,
  /class="quote-card__mark">”<\/text>/,
  '学习页语录卡片需要右侧双引号水印标识',
)

assert.match(
  learnSource,
  /\.quote-card__mark\s*\{[^}]*position:\s*absolute/s,
  '双引号水印需要绝对定位在卡片内',
)

console.log('miniapp learn quote card tests passed')
