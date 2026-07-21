import { formatFileSizeMiB, formatLocalDate, formatSHA256 } from './appDownloadDevice.js'

const KNOWN_DEVICES = new Set(['android', 'ios', 'desktop'])

function getState({ error, loading, release }) {
  if (loading) return 'loading'
  if (error && Number(error.status) === 503) return 'missing-file'
  if (error) return 'error'
  if (release?.available) return 'published'
  return 'unavailable'
}

function getStatusMessage(state, config) {
  if (state === 'loading') return '正在获取 Android 最新版本…'
  if (state === 'published') return 'Android 最新正式版本已发布'
  if (state === 'missing-file') return '安装包暂时不可用'
  if (state === 'error') return '版本信息加载失败，请稍后重试'
  return config.unavailableText || 'Android 版本暂未开放下载'
}

function buildQRPayload(pageURL) {
  try {
    const { origin } = new URL(pageURL)
    return origin === 'null' ? '/#download-app' : `${origin}/#download-app`
  } catch {
    return '/#download-app'
  }
}

export function createAppDownloadViewModel({
  config = {},
  device,
  error,
  loading,
  release,
  downloadURL = '',
  pageURL = '',
}) {
  const normalizedDevice = KNOWN_DEVICES.has(device) ? device : 'desktop'
  const state = getState({ error, loading, release })
  const published = state === 'published'
  const isIOS = normalizedDevice === 'ios'
  const showQRCode = published && normalizedDevice === 'desktop'

  return {
    device: normalizedDevice,
    state,
    loading: state === 'loading',
    statusMessage: getStatusMessage(state, config),
    canRetry: state === 'missing-file' || state === 'error',
    retryText: config.retryText || '重新加载',
    eyebrow: config.eyebrow || '',
    appName: config.title || '',
    introduction: config.lead || '',
    features: Array.isArray(config.features) ? config.features : [],
    installSteps: Array.isArray(config.installSteps) ? config.installSteps : [],
    showPublishedMetadata: published,
    version: published && release.versionName ? release.versionName : '—',
    publishTime: published ? formatLocalDate(release.publishedAt) : '—',
    fileSize: published ? formatFileSizeMiB(release.fileSize) : '—',
    updateNotes: published && release.releaseNotes?.trim()
      ? release.releaseNotes.trim()
      : '暂无更新说明',
    sha256: published ? formatSHA256(release.sha256) : '—',
    showDownloadAction: published,
    actionLabel: isIOS
      ? (config.iosComingSoonText || 'iOS 敬请期待')
      : (config.androidButtonText || '下载 Android 版'),
    actionHref: published && !isIOS ? downloadURL : '',
    actionDisabled: !published || isIOS || !downloadURL,
    showQRCode,
    qrPayload: showQRCode ? buildQRPayload(pageURL) : '',
  }
}
