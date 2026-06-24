import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import { TYPE_DETAILS } from '../data/typeDetails.js'
import { FEATURED_VIDEOS, VIDEOS } from '../data/videos.js'

const __dirname = dirname(fileURLToPath(import.meta.url))
const appSource = readFileSync(resolve(__dirname, '../App.jsx'), 'utf8')
const homeSource = readFileSync(resolve(__dirname, 'Home.jsx'), 'utf8')
const homeSectionsSource = readFileSync(resolve(__dirname, 'homeSections.jsx'), 'utf8')
const courseSource = readFileSync(resolve(__dirname, 'Course.jsx'), 'utf8')
const watchSource = readFileSync(resolve(__dirname, 'Watch.jsx'), 'utf8')
const cssSource = readFileSync(resolve(__dirname, '../index.css'), 'utf8')

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

assert.match(watchSource, /import\s*\{\s*VIDEOS\s*\}\s*from\s*'..\/data\/videos'/, '观看页需要使用共享视频列表数据')
assert.match(watchSource, /<video[\s\S]*controls/, '观看页需要真实 video 播放器')
assert.match(watchSource, /video-list/, '观看页需要视频列表')
assert.match(watchSource, /useRef/, '观看页需要持有主播放器 ref')
assert.match(watchSource, /useEffect[\s\S]*activeId[\s\S]*\.play\(\)/, '点击切换视频后需要自动播放当前视频')
assert.match(watchSource, /\.catch\(\(\)\s*=>\s*\{\}\)/, '浏览器阻止自动播放时需要静默处理，避免控制台报错')
assert.match(watchSource, /dispatchEvent\(new CustomEvent\(['"]site:pause-music['"]\)\)/, '主视频开始播放时需要关闭背景音乐')

assert.equal(VIDEOS.length, 19, '观看页需要去掉重复内容后展示 19 个精选视频')
assert.equal(FEATURED_VIDEOS.length, 3, '首页只展示 3 个精选视频，避免首屏过重')
assert.equal(new Set(VIDEOS.map((video) => video.src)).size, VIDEOS.length, '视频播放源不能重复')
for (const video of VIDEOS) {
  assert.match(video.src, /^\/assets\/videos\/laohan-\d{2}\.mp4$/, `${video.id} 需要指向 public/assets/videos 下的 mp4`)
  assert.match(video.poster, /^\/assets\/videos\/posters\/laohan-\d{2}\.jpg$/, `${video.id} 需要使用对应视频抽帧封面`)
  assert.notEqual(video.poster, '/assets/teacher-poster.jpg', `${video.id} 不应再使用老师海报做视频封面`)
  assert.ok(video.duration, `${video.id} 需要展示视频时长`)
}
assert.equal(new Set(VIDEOS.map((video) => video.poster)).size, VIDEOS.length, '每个视频需要有不同封面，避免列表看起来都一样')
for (const video of FEATURED_VIDEOS) {
  assert.ok(VIDEOS.some((item) => item.id === video.id), `精选视频 ${video.id} 必须来自完整视频列表`)
}

assert.match(homeSource, /FEATURED_VIDEOS/, '首页需要读取精选视频列表')
assert.match(homeSource, /home-video/, '首页需要在视频位置渲染精选视频区域')
assert.match(homeSource, /to="\/watch"[\s\S]*更多视频/, '首页视频区域右上角需要有“更多视频”入口')
assert.match(homeSource, /className="home-video-card"[\s\S]*<img/, '首页精选视频需要用独立封面展示，避免多个播放器看起来重复')
assert.doesNotMatch(homeSource, /<video[\s\S]*home-video-card|home-video-card[\s\S]*<video/, '首页精选区不应直接嵌入多个 video 播放器')
assert.match(watchSource, /watch__player-column/, '更多视频页需要独立的播放器列')
assert.match(watchSource, /watch__list-column/, '更多视频页需要独立的视频列表列')
assert.match(watchSource, /video-list__grid/, '更多视频页列表需要使用网格布局，移动端排版不能拥挤')
assert.match(cssSource, /\.watch__player-column\s*\{[\s\S]*position:\s*sticky;[\s\S]*top:\s*92px;/, '桌面端播放器需要固定在视口内')
assert.match(cssSource, /\.watch__grid\s*\{[\s\S]*grid-template-columns:\s*minmax\(280px,\s*380px\)\s*minmax\(320px,\s*440px\);/, '桌面端更多视频页需要播放器和列表保持紧凑比例')
assert.match(cssSource, /\.watch\s*\{[\s\S]*--watch-player-w:\s*min\(100%,\s*360px,\s*calc\(\(100dvh\s*-\s*220px\)\s*\*\s*9\s*\/\s*16\)\);/, '主播放器需要按视口高度约束，桌面端一屏可完整展示')
assert.match(cssSource, /\.video-list__grid\s*\{[\s\S]*grid-template-columns:\s*1fr;/, '桌面端更多视频列表应为紧凑单列')
assert.match(cssSource, /\.video-list__grid\s*\{[\s\S]*overflow-y:\s*auto;/, '桌面端更多视频列表需要独立滚动，不能带动主播放器')
assert.match(cssSource, /\.video-list__grid\s*\{[\s\S]*gap:\s*12px;/, '视频列表项之间需要有足够间隔，避免选中态贴住下一条')
assert.match(cssSource, /\.video-item\.active\s*\{[\s\S]*border-left-color:\s*var\(--blue\);/, '选中视频需要使用明确的左侧高亮条')
assert.match(cssSource, /\.video-item\s+img\s*\{[\s\S]*width:\s*54px;[\s\S]*height:\s*60px;/, '视频列表缩略图需要固定尺寸，避免插入行内造成错位')
assert.match(cssSource, /@media\s*\(max-width:\s*920px\)[\s\S]*\.video-list__grid\s*\{[\s\S]*max-height:\s*none;[\s\S]*overflow-y:\s*visible;/, '移动端需要恢复页面自然滚动')
assert.match(cssSource, /@media\s*\(max-width:\s*920px\)[\s\S]*\.video-list__grid\s*\{[\s\S]*grid-template-columns:\s*1fr;/, '移动端视频列表需要保持单列，避免标题被挤压')
assert.match(cssSource, /@media\s*\(max-width:\s*920px\)[\s\S]*--watch-player-w:\s*min\(100%,\s*330px,\s*calc\(\(100dvh\s*-\s*180px\)\s*\*\s*9\s*\/\s*16\)\);/, '移动端主播放器需要按手机视口缩放，一屏可完整展示')

console.log('website feature flow tests passed')
