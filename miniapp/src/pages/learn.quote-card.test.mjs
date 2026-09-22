import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const learnSource = readFileSync(resolve(__dirname, 'learn/learn.vue'), 'utf8')

assert.match(
  learnSource,
  /const\s+quotes\s*=\s*ref\(initialContent\.quotes\)/,
  '学习页需要读取官网语录配置，新增语录才能同步显示',
)

assert.match(
  learnSource,
  /const\s+quoteEntries\s*=\s*computed\(\(\)\s*=>\s*createLearningQuoteEntries\(quotes\.value\)\)/,
  '学习页需要为重复语录生成稳定唯一键',
)

assert.match(
  learnSource,
  /<article\s+v-for="quoteEntry in quoteEntries"\s+:key="quoteEntry\.key"\s+class="quote-card">/,
  '学习页需要使用统一的 quote-card 语录容器',
)

assert.match(
  learnSource,
  /<text\s+class="quote-card__mark"\s+aria-hidden="true">”<\/text>/,
  '学习页语录需要展示不重复朗读的编辑式引号标识',
)

assert.match(
  learnSource,
  /<text\s+class="quote-card__text">\{\{ quoteEntry\.text \}\}<\/text>/,
  '学习页语录正文需要原样渲染配置文本，不能额外拼接引号',
)

assert.match(
  learnSource,
  /\.learning-panel\s*\{[^}]*background:\s*var\(--nx-surface\)/s,
  '语录面板需要使用统一的阅读表面色',
)

assert.match(learnSource, /class="learn-header__title">学习中心<\/text>/, '学习页需要显示当前学习中心标题')
assert.doesNotMatch(
  learnSource,
  /#4338ca|#4f46e5|#7c3aed|#f59e0b/i,
  '学习页语录样式不应携带旧紫色或橙色主视觉',
)

console.log('miniapp learn quote card tests passed')
