import assert from 'node:assert/strict'
import test from 'node:test'

import { createAppDownloadViewModel } from './appDownloadViewModel.js'

const config = {
  eyebrow: '官方 App',
  title: '九型芯之力 App',
  lead: '把课程、练习与成长内容装进口袋。',
  features: ['官方版本', '随时学习', '持续更新'],
  installSteps: ['下载 APK', '允许本次安装', '按提示完成安装'],
  androidButtonText: '下载 Android 版',
  iosComingSoonText: 'iOS 敬请期待',
  unavailableText: 'Android 版本暂未开放下载',
  retryText: '重新加载',
}

const release = {
  available: true,
  versionName: '1.4.0',
  versionCode: 10400,
  publishedAt: '2026-07-20T08:30:00Z',
  fileSize: 15.25 * 1024 * 1024,
  releaseNotes: '新增课程离线阅读。',
  sha256: '0123456789abcdef'.repeat(4),
}

function build(overrides = {}) {
  return createAppDownloadViewModel({
    config,
    device: 'desktop',
    error: null,
    loading: false,
    release,
    downloadURL: '/api/public/app-release/download?platform=android',
    pageURL: 'https://www.xinzhili.example/learn?from=qr',
    ...overrides,
  })
}

test('exposes a stable loading state without a download action', () => {
  const model = build({ loading: true, release: null })

  assert.equal(model.state, 'loading')
  assert.equal(model.loading, true)
  assert.equal(model.showPublishedMetadata, false)
  assert.equal(model.showDownloadAction, false)
  assert.equal(model.canRetry, false)
})

test('exposes all published metadata and configured product copy', () => {
  const model = build()

  assert.equal(model.state, 'published')
  assert.equal(model.eyebrow, config.eyebrow)
  assert.equal(model.appName, config.title)
  assert.equal(model.introduction, config.lead)
  assert.deepEqual(model.features, config.features)
  assert.deepEqual(model.installSteps, config.installSteps)
  assert.equal(model.version, release.versionName)
  assert.equal(model.publishTime, new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(release.publishedAt)))
  assert.equal(model.fileSize, '15.3 MiB')
  assert.equal(model.updateNotes, release.releaseNotes)
  assert.match(model.sha256, /^01234567 89abcdef/)
  assert.equal(model.showPublishedMetadata, true)
})

test('uses the configured unavailable state when no release is published', () => {
  const model = build({ release: { available: false } })

  assert.equal(model.state, 'unavailable')
  assert.equal(model.statusMessage, config.unavailableText)
  assert.equal(model.showDownloadAction, false)
  assert.equal(model.showQRCode, false)
  assert.equal(model.canRetry, false)
})

test('uses a dedicated recoverable message for a missing package response', () => {
  const model = build({ error: { status: 503 }, release: null })

  assert.equal(model.state, 'missing-file')
  assert.equal(model.statusMessage, '安装包暂时不可用')
  assert.equal(model.canRetry, true)
  assert.equal(model.retryText, config.retryText)
  assert.equal(model.showDownloadAction, false)
})

test('uses a safe retry state for ordinary network errors', () => {
  const model = build({ error: { status: 0, message: 'ECONNREFUSED 10.0.0.8' }, release: null })

  assert.equal(model.state, 'error')
  assert.equal(model.statusMessage, '版本信息加载失败，请稍后重试')
  assert.doesNotMatch(model.statusMessage, /ECONNREFUSED|10\.0\.0\.8/)
  assert.equal(model.canRetry, true)
  assert.equal(model.retryText, config.retryText)
})

test('gives Android devices a direct latest-download link without QR', () => {
  const model = build({ device: 'android' })

  assert.equal(model.device, 'android')
  assert.equal(model.actionLabel, config.androidButtonText)
  assert.equal(model.actionHref, '/api/public/app-release/download?platform=android')
  assert.equal(model.actionDisabled, false)
  assert.equal(model.showDownloadAction, true)
  assert.equal(model.showQRCode, false)
})

test('gives iOS devices a real disabled coming-soon action', () => {
  const model = build({ device: 'ios' })

  assert.equal(model.actionLabel, config.iosComingSoonText)
  assert.equal(model.actionHref, '')
  assert.equal(model.actionDisabled, true)
  assert.equal(model.showDownloadAction, true)
  assert.equal(model.showQRCode, false)
})

test('gives desktop devices the Android action and an on-site QR payload', () => {
  const model = build({ device: 'desktop' })

  assert.equal(model.actionHref, '/api/public/app-release/download?platform=android')
  assert.equal(model.showQRCode, true)
  assert.equal(model.qrPayload, 'https://www.xinzhili.example/#download-app')
  assert.doesNotMatch(model.qrPayload, /qrserver|google|third-party/i)
})

test('treats unknown devices as desktop', () => {
  const model = build({ device: 'smart-fridge' })

  assert.equal(model.device, 'desktop')
  assert.equal(model.showQRCode, true)
  assert.equal(model.actionDisabled, false)
})
