import assert from 'node:assert/strict'
import { existsSync, readFileSync, statSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const readSource = (path) => {
  const absolutePath = resolve(__dirname, path)
  return existsSync(absolutePath) ? readFileSync(absolutePath, 'utf8') : ''
}

const appSource = readSource('../App.jsx')
const pageSource = readSource('./AppDownload.jsx')
const contentSource = readSource('../data/appDownloadPage.js')
const componentSource = readSource('../components/AppDownloadSection.jsx')
const layoutSource = readSource('../components/Layout.jsx')
const configModuleSource = readSource('../data/siteConfig.js')
const siteConfig = JSON.parse(readSource('../../../shared/site-config.json'))

test('registers a lazy /app route and points every primary App entry to it', () => {
  assert.match(appSource, /const AppDownload = lazy\(\(\) => import\('\.\/pages\/AppDownload'\)\)/)
  assert.match(appSource, /<Route path="app" element=\{lazyRoute\(<AppDownload \/>\)\} \/>/)

  for (const collection of [siteConfig.navigation.main, siteConfig.navigation.drawer]) {
    assert.ok(collection.some((item) => (
      item.label === '下载 App'
      && item.to === '/app'
      && item.type === 'route'
    )))
  }

  assert.ok(siteConfig.home.hero.actions.some((item) => (
    item.label === '下载 App'
    && item.to === '/app'
    && item.type === 'route'
  )))
  assert.match(componentSource, /showDetailsLink/)
  assert.match(componentSource, /to="\/app"/)
  assert.match(layoutSource, /const isAppPage = location\.pathname\.replace\(/)
  assert.match(layoutSource, /!isAppPage && <Music \/>/)
  assert.match(layoutSource, /!isAppPage && <CustomerService \/>/)
  assert.match(layoutSource, /!isAppPage && <Tabbar \/>/)
  assert.match(configModuleSource, /isProtectedAppEntry/)
  assert.match(configModuleSource, /keepProtectedAppEntries/)
  assert.match(configModuleSource, /ensureProtectedAppEntry/)
  assert.match(configModuleSource, /merged\.home\.appDownload\s*=\s*structuredClone\(bundledConfig\.home\.appDownload\)/)
  assert.match(configModuleSource, /navigation\[key\]\s*=\s*\[\]/)
  assert.match(configModuleSource, /merged\.home\.hero\.actions\s*=\s*\[\]/)
})

test('builds the dedicated page around download, features, install guidance, and release history', () => {
  assert.match(pageSource, /<AppDownloadSection\s+showInstallSummary=\{false\}\s*\/>/)
  assert.match(pageSource, /id="app-features"/)
  assert.match(pageSource, /id="install-guide"/)
  assert.match(pageSource, /id="release-notes"/)
  assert.match(pageSource, /APP_PAGE_FEATURES\.map/)
  assert.match(pageSource, /APP_INSTALL_STEPS\.map/)
  assert.match(pageSource, /APP_RELEASE_HISTORY\.map/)
  assert.match(contentSource, /智能成长对话/)
  assert.match(contentSource, /关系洞察/)
  assert.match(contentSource, /人生故事/)
  assert.ok(
    pageSource.indexOf('className="app-page__steps-column"')
      < pageSource.indexOf('className="app-page__video-column"'),
    '移动端视觉顺序必须与 DOM 和键盘顺序一致：先步骤，后视频',
  )
})

test('uses a real App screenshot and page-specific search metadata', () => {
  const preview = resolve(__dirname, '../../public/assets/app/app-home-preview.webp')

  assert.ok(existsSync(preview), '首屏需要使用真实 App 界面截图')
  assert.ok(statSync(preview).size > 10 * 1024, 'App 界面截图文件大小异常')
  assert.match(contentSource, /APP_HERO_PREVIEW/)
  assert.match(pageSource, /src=\{APP_HERO_PREVIEW\.src\}/)
  assert.match(pageSource, /alt=\{APP_HERO_PREVIEW\.alt\}/)
  assert.doesNotMatch(pageSource, /app-page__device-lines/)
  assert.match(pageSource, /document\.title\s*=\s*APP_PAGE_META\.title/)
  assert.match(pageSource, /meta\[name="description"\]/)
  assert.match(pageSource, /meta\[property="og:title"\]/)
  assert.match(pageSource, /meta\[property="og:description"\]/)
})

test('bundles the supplied installation video with an accessible poster and captions', () => {
  const video = resolve(__dirname, '../../public/assets/app/install-guide.mp4')
  const poster = resolve(__dirname, '../../public/assets/app/install-guide-poster.webp')
  const captions = resolve(__dirname, '../../public/assets/app/install-guide.zh-CN.vtt')

  assert.ok(existsSync(video), '安装视频需要随官网静态资源发布')
  assert.ok(existsSync(poster), '安装视频需要提供稳定的海报图以避免布局跳动')
  assert.ok(existsSync(captions), '安装视频需要提供简体中文字幕轨道')
  assert.ok(statSync(video).size > 1024 * 1024, '安装视频文件大小异常，请确认已复制完整视频')
  assert.ok(statSync(poster).size > 1024, '安装视频海报文件大小异常')
  assert.match(readFileSync(captions, 'utf8'), /^WEBVTT[\s\S]*00:00\.000[\s\S]*03:31\.000/)
  assert.match(pageSource, /<video[\s\S]*controls[\s\S]*playsInline[\s\S]*preload="metadata"[\s\S]*poster=\{APP_INSTALL_VIDEO\.poster\}/)
  assert.match(pageSource, /<track[\s\S]*kind="captions"[\s\S]*srcLang="zh-CN"[\s\S]*default/)
  assert.match(pageSource, /window\.dispatchEvent\(new CustomEvent\('site:pause-music'\)\)/)
  assert.match(pageSource, /如果视频无法播放[\s\S]*href=\{APP_INSTALL_VIDEO\.src\}/)
})

test('keeps version history factual and tells visitors that live metadata wins', () => {
  assert.match(contentSource, /最新版本与下载信息以上方实时数据为准/)
  assert.match(contentSource, /历史版本记录/)
  assert.doesNotMatch(contentSource, /version:\s*'1\.1\.12'/)
  assert.doesNotMatch(contentSource, /最近更新/)
})
