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
  /<view\s+v-for="quote in quotes"\s+:key="quote"\s+class="quote-editorial">/,
  '学习页需要使用批准的 quote-editorial 语录容器',
)

assert.match(
  learnSource,
  /<text\s+class="quote-editorial__mark"\s+aria-hidden="true">“<\/text>/,
  '学习页语录需要展示不重复朗读的编辑式引号标识',
)

assert.match(
  learnSource,
  /<text\s+class="quote-editorial__text">\{\{ quote \}\}<\/text>/,
  '学习页语录正文需要原样渲染配置文本，不能额外拼接引号',
)

assert.match(
  learnSource,
  /\.quote-editorial\s*\{[^}]*background:\s*var\(--nx-surface-soft\)/s,
  '编辑式语录需要使用统一的暖白阅读底色',
)

assert.match(learnSource, /老师课堂/, '学习页需要把课堂入口作为页面主标题')
assert.doesNotMatch(
  learnSource,
  /#4338ca|#4f46e5|#7c3aed|#f59e0b/i,
  '学习页语录样式不应携带旧紫色或橙色主视觉',
)

console.log('miniapp learn quote card tests passed')
