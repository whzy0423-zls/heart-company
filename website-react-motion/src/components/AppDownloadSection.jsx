import { useEffect, useState } from 'react'
import QRCode from 'qrcode'
import {
  buildLatestAppReleaseDownloadURL,
  getLatestAppRelease,
} from '../api/appRelease'
import siteConfig from '../data/siteConfig'
import { detectAppDownloadDevice } from '../utils/appDownloadDevice'
import { createAppDownloadViewModel } from '../utils/appDownloadViewModel'

export default function AppDownloadSection() {
  const [device] = useState(() => detectAppDownloadDevice())
  const [release, setRelease] = useState(null)
  const [requestError, setRequestError] = useState(null)
  const [loading, setLoading] = useState(true)
  const [requestRevision, setRequestRevision] = useState(0)
  const [qrCodeDataURL, setQRCodeDataURL] = useState('')
  const [qrCodeError, setQRCodeError] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setRequestError(null)
    setRelease(null)

    getLatestAppRelease({ signal: controller.signal })
      .then((nextRelease) => {
        if (!controller.signal.aborted) setRelease(nextRelease)
      })
      .catch((error) => {
        const requestError = error
        if (requestError?.name === 'AbortError' || controller.signal.aborted) return
        setRequestError(requestError)
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })

    return () => controller.abort()
  }, [requestRevision])

  const pageURL = typeof window === 'undefined' ? '' : window.location.href
  const viewModel = createAppDownloadViewModel({
    config: siteConfig.home.appDownload,
    device,
    error: requestError,
    loading,
    release,
    downloadURL: buildLatestAppReleaseDownloadURL(),
    pageURL,
  })

  useEffect(() => {
    let active = true

    if (!viewModel.showQRCode || !viewModel.qrPayload) {
      setQRCodeDataURL('')
      setQRCodeError(false)
      return () => { active = false }
    }

    setQRCodeDataURL('')
    setQRCodeError(false)
    QRCode.toDataURL(viewModel.qrPayload, {
      width: 224,
      margin: 1,
      color: {
        dark: '#12151b',
        light: '#ffffff',
      },
    })
      .then((dataURL) => {
        if (active) setQRCodeDataURL(dataURL)
      })
      .catch(() => {
        if (!active) return
        setQRCodeDataURL('')
        setQRCodeError(true)
      })

    return () => { active = false }
  }, [viewModel.qrPayload, viewModel.showQRCode])

  const retry = () => setRequestRevision((revision) => revision + 1)

  return (
    <section
      className={`app-download app-download--${viewModel.state}`}
      id="download-app"
      aria-labelledby="app-download-title"
    >
      <div className="wrap app-download__inner">
        <header className="app-download__heading">
          <p className="eyebrow">{viewModel.eyebrow}</p>
          <h2 className="section-title" id="app-download-title">{viewModel.appName}</h2>
          <p className="lead">{viewModel.introduction}</p>
        </header>

        <div className="app-download__grid">
          <div className="app-download__content">
            <div className="app-download__brand-row">
              <span className="app-download__logo">
                <img
                  src={siteConfig.site.logo}
                  alt={`${viewModel.appName} 标志`}
                  width="76"
                  height="76"
                />
              </span>
              <div>
                <p className="app-download__overline">Android 正式版</p>
                <h3>{viewModel.appName}</h3>
              </div>
            </div>

            <ul className="app-download__features">
              {viewModel.features.map((feature) => <li key={feature}>{feature}</li>)}
            </ul>

            <div
              className={`app-download__status app-download__status--${viewModel.state}`}
              aria-live="polite"
              aria-busy={viewModel.loading}
            >
              <span aria-hidden="true" />
              <p>{viewModel.statusMessage}</p>
            </div>

            {viewModel.showPublishedMetadata ? (
              <div className="app-download__release">
                <dl className="app-download__facts">
                  <div>
                    <dt>版本</dt>
                    <dd>{viewModel.version}</dd>
                  </div>
                  <div>
                    <dt>发布时间</dt>
                    <dd>{viewModel.publishTime}</dd>
                  </div>
                  <div>
                    <dt>文件大小</dt>
                    <dd>{viewModel.fileSize}</dd>
                  </div>
                </dl>
                <div className="app-download__notes">
                  <h4>更新说明</h4>
                  <p>{viewModel.updateNotes}</p>
                </div>
                <div className="app-download__digest">
                  <h4>SHA-256</h4>
                  <code className="app-download__sha">{viewModel.sha256}</code>
                </div>
              </div>
            ) : (
              <div className="app-download__release-placeholder" aria-hidden="true">
                <span />
                <span />
                <span />
              </div>
            )}
          </div>

          <aside className="app-download__aside" aria-label="App 下载与安装">
            <div className="app-download__action-area">
              {viewModel.showDownloadAction ? (
                viewModel.actionDisabled ? (
                  <button className="app-download__action" type="button" disabled>
                    {viewModel.actionLabel}
                  </button>
                ) : (
                  <a className="app-download__action" href={viewModel.actionHref}>
                    {viewModel.actionLabel}
                  </a>
                )
              ) : (
                <div className="app-download__action-placeholder">
                  {viewModel.statusMessage}
                </div>
              )}

              {viewModel.canRetry && (
                <button className="app-download__retry" type="button" onClick={retry}>
                  {viewModel.retryText}
                </button>
              )}
            </div>

            <div className="app-download__qr-space">
              {viewModel.showQRCode ? (
                <>
                  <div className="app-download__qr-frame">
                    {qrCodeDataURL && (
                      <img
                        src={qrCodeDataURL}
                        alt="App 下载页面二维码"
                        width="224"
                        height="224"
                      />
                    )}
                    {!qrCodeDataURL && !qrCodeError && (
                      <span className="app-download__qr-loading">正在生成二维码</span>
                    )}
                    {qrCodeError && (
                      <span className="app-download__qr-fallback">
                        二维码暂时无法生成，请使用下载按钮
                      </span>
                    )}
                  </div>
                  <p>手机扫码打开官网下载页</p>
                </>
              ) : (
                <div className="app-download__device-note">
                  {viewModel.device === 'ios'
                    ? 'iOS 版本正在准备中，产品介绍与版本动态会在此同步。'
                    : '下载后请按照下方步骤完成 Android 安装。'}
                </div>
              )}
            </div>

            <div className="app-download__install">
              <h3>安装步骤</h3>
              <ol>
                {viewModel.installSteps.map((step) => <li key={step}>{step}</li>)}
              </ol>
            </div>
          </aside>
        </div>
      </div>
    </section>
  )
}
