import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const readSource = (path) => {
  const absolutePath = resolve(__dirname, path)
  return existsSync(absolutePath) ? readFileSync(absolutePath, 'utf8') : ''
}

const componentSource = readSource('./AppDownloadSection.jsx')
const viewModelSource = readSource('../utils/appDownloadViewModel.js')
const layoutSource = readSource('./Layout.jsx')
const cssSource = readSource('../index.css')
const siteConfig = JSON.parse(readSource('../../../shared/site-config.json'))

test('loads metadata with abort-safe lifecycle handling and builder-only download URLs', () => {
  assert.match(componentSource, /getLatestAppRelease/)
  assert.match(componentSource, /buildLatestAppReleaseDownloadURL/)
  assert.match(componentSource, /createAppDownloadViewModel/)
  assert.match(componentSource, /new AbortController\(\)/)
  assert.match(componentSource, /signal:\s*controller\.signal/)
  assert.match(componentSource, /controller\.abort\(\)/)
  assert.match(componentSource, /(?:error|requestError)\?\.name\s*===\s*['"]AbortError['"]/)
  assert.doesNotMatch(componentSource, /['"]\/api\/public\/app-release\/download/)
  assert.doesNotMatch(componentSource, /location\.(?:assign|replace)\(|location\.href\s*=|\.click\(\)/)
})

test('generates the on-site QR lazily and renders a failure fallback', () => {
  assert.match(componentSource, /import QRCode from ['"]qrcode['"]/)
  assert.match(componentSource, /useEffect\([\s\S]*QRCode\.toDataURL\(/)
  assert.match(componentSource, /QRCode\.toDataURL\(viewModel\.qrPayload/)
  assert.match(componentSource, /\.catch\([\s\S]*setQRCodeError/)
  assert.match(componentSource, /二维码暂时无法生成，请使用下载按钮/)
})

test('renders product copy, every published field, installation steps, and official branding', () => {
  for (const field of [
    'appName',
    'introduction',
    'version',
    'publishTime',
    'fileSize',
    'updateNotes',
    'sha256',
  ]) {
    assert.match(componentSource, new RegExp(`viewModel\\.${field}`), `${field} 需要由 view model 渲染`)
  }

  assert.match(componentSource, /viewModel\.installSteps\.map/)
  assert.match(componentSource, /版本/)
  assert.match(componentSource, /发布时间/)
  assert.match(componentSource, /文件大小/)
  assert.match(componentSource, /更新说明/)
  assert.match(componentSource, /SHA-256/)
  assert.match(componentSource, /安装步骤/)
  assert.match(componentSource, /siteConfig\.site\.logo/)
  assert.equal(siteConfig.site.logo, '/assets/logo.svg')
  assert.doesNotMatch(componentSource, /[\u{1F300}-\u{1FAFF}]/u)
})

test('renders accessible device actions, 503 feedback, and retry controls', () => {
  assert.match(componentSource, /id="download-app"/)
  assert.match(componentSource, /aria-live="polite"/)
  assert.match(componentSource, /viewModel\.actionDisabled[\s\S]*<button[\s\S]*disabled/)
  assert.match(componentSource, /viewModel\.showQRCode/)
  assert.match(componentSource, /viewModel\.canRetry/)
  assert.match(componentSource, /viewModel\.retryText/)
  assert.match(viewModelSource, /安装包暂时不可用/)
})

test('reserves a responsive no-overflow footprint with accessible interactions', () => {
  assert.match(
    cssSource,
    /\.app-download\s*\{[^}]*max-width:\s*100%;[^}]*overflow-x:\s*(?:clip|hidden);[^}]*scroll-margin-top:\s*(?:9[4-9]|[1-9]\d{2,})px;/s,
  )
  assert.match(cssSource, /\.app-download__content\s*\{[^}]*min-height:/s)
  assert.match(cssSource, /\.app-download__action[^}]*\{[^}]*min-height:\s*44px;/s)
  assert.match(cssSource, /\.app-download__action:focus-visible\s*\{[^}]*outline:/s)
  assert.match(
    cssSource,
    /\.app-download__sha\s*\{[^}]*font-family:[^}]*monospace[^}]*overflow-wrap:\s*anywhere;/s,
  )
  assert.match(cssSource, /\.app-download__grid\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*[^)]+\)\s+minmax\(0,\s*[^)]+\);/s)
  assert.match(
    cssSource,
    /@media\s*\(max-width:\s*(?:760|768)px\)[\s\S]*\.app-download__grid\s*\{[^}]*grid-template-columns:\s*1fr;/s,
  )
  assert.match(cssSource, /\.app-download__action\s*\{[^}]*transition:\s*(?=[^;}]*\.2s)(?=[^;}]*(?:transform|opacity))/s)
  assert.match(
    cssSource,
    /@media\s*\(prefers-reduced-motion:\s*reduce\)[\s\S]*\.app-download__action[^}]*\{[^}]*transition:\s*none;/s,
  )
})

test('uses reduced-motion-aware hash scrolling in the global layout', () => {
  assert.match(layoutSource, /matchMedia\(['"]\(prefers-reduced-motion: reduce\)['"]\)/)
  assert.match(layoutSource, /behavior:\s*reduceMotion\s*\?\s*['"]auto['"]\s*:\s*['"]smooth['"]/)
})

test('disables native smooth scrolling for reduced-motion users', () => {
  assert.match(
    cssSource,
    /@media\s*\(prefers-reduced-motion:\s*reduce\)[\s\S]*html\s*\{[^}]*scroll-behavior:\s*auto;/s,
  )
})

test('reserves the published QR footprint across mobile loading and error states', () => {
  assert.match(
    cssSource,
    /\.app-download__qr-space\s*\{[^}]*min-height:\s*26\dpx;/s,
  )
  assert.match(
    cssSource,
    /@media\s*\(max-width:\s*(?:760|768)px\)[\s\S]*\.app-download__qr-space\s*\{[^}]*min-height:\s*26\dpx;/s,
  )
})
