import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import { TYPE_DETAILS } from '../data/typeDetails.js'

const __dirname = dirname(fileURLToPath(import.meta.url))
const appSource = readFileSync(resolve(__dirname, '../App.jsx'), 'utf8')
const homeSource = readFileSync(resolve(__dirname, 'Home.jsx'), 'utf8')
const homeSectionsSource = readFileSync(resolve(__dirname, 'homeSections.jsx'), 'utf8')
const courseSource = readFileSync(resolve(__dirname, 'Course.jsx'), 'utf8')
const watchSource = readFileSync(resolve(__dirname, 'Watch.jsx'), 'utf8')

assert.equal(TYPE_DETAILS.length, 9, '九型详情需要覆盖 1-9 号全部型号')
for (const detail of TYPE_DETAILS) {
  assert.ok(detail.id >= 1 && detail.id <= 9, `型号 ${detail.id} id 必须在 1-9`)
  assert.ok(detail.intro.length >= 40, `${detail.id} 号需要有完整介绍文案`)
  assert.ok(detail.bestFor.length >= 3, `${detail.id} 号必须包含至少 3 条适合做什么`)
  assert.ok(detail.notFor.length >= 3, `${detail.id} 号必须包含至少 3 条不适合做什么`)
}

assert.match(appSource, /path="type\/:id"/, '官网需要 /type/:id 型号详情路由')
assert.match(homeSectionsSource, /to=\{`\/type\/\$\{t\[0\]\}`\}/, '首页九型卡片点击需要进入型号详情')
assert.match(homeSectionsSource, /className="card type-card type-card--link"/, '可点击型号卡片需要明确的链接态样式')

assert.match(courseSource, /course__slider/, '课件需要使用滑动轨道展示')
assert.match(courseSource, /onScroll=\{handleSlideScroll\}/, '课件滑动时需要同步当前页和进度')
assert.doesNotMatch(courseSource, /course__nav--next/, '课件不应再依赖点击下一页按钮作为主交互')

assert.match(watchSource, /const\s+VIDEOS\s*=\s*\[/, '观看页需要视频列表数据')
assert.match(watchSource, /<video[\s\S]*controls/, '观看页需要真实 video 播放器')
assert.match(watchSource, /video-list/, '观看页需要视频列表')

console.log('website feature flow tests passed')
