import { useEffect } from 'react'
import AppDownloadSection from '../components/AppDownloadSection'
import Reveal from '../components/Reveal'
import siteConfig from '../data/siteConfig'
import {
  APP_HERO_PREVIEW,
  APP_INSTALL_STEPS,
  APP_INSTALL_VIDEO,
  APP_PAGE_META,
  APP_PAGE_FEATURES,
  APP_RELEASE_DISCLAIMER,
  APP_RELEASE_HISTORY,
} from '../data/appDownloadPage'

const ICONS = {
  chat: 'M5 17.5 3.5 21l4.4-1.9c1.2.6 2.6.9 4.1.9 5 0 9-3.6 9-8s-4-8-9-8-9 3.6-9 8c0 2.1.9 4 2.5 5.5Z M8 11h.01M12 11h.01M16 11h.01',
  portrait: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8ZM4.5 21c.8-4.1 3.8-6.5 7.5-6.5s6.7 2.4 7.5 6.5',
  relationship: 'M8.5 12a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7ZM15.5 11a3 3 0 1 0 0-6M2.5 20c.6-3.5 2.9-5.5 6-5.5s5.4 2 6 5.5M15 14.5c3.1 0 5.3 2 5.8 5.5',
  story: 'M5 4.5h11.5A2.5 2.5 0 0 1 19 7v12.5H7.5A2.5 2.5 0 0 1 5 17V4.5ZM8 8h8M8 11.5h8M8 15h5',
  practice: 'M9 3h6l1 2h3v16H5V5h3l1-2ZM9 12l2 2 4-5',
  library: 'M4 5.5A2.5 2.5 0 0 1 6.5 3H11v17H6.5A2.5 2.5 0 0 0 4 22V5.5ZM20 5.5A2.5 2.5 0 0 0 17.5 3H13v17h4.5A2.5 2.5 0 0 1 20 22V5.5Z',
}

function FeatureIcon({ name }) {
  return (
    <svg viewBox="0 0 24 24" width="25" height="25" fill="none" stroke="currentColor"
         strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={ICONS[name]} />
    </svg>
  )
}

export default function AppDownload() {
  useEffect(() => {
    const previousTitle = document.title
    const metadata = [
      ['meta[name="description"]', APP_PAGE_META.description],
      ['meta[property="og:title"]', APP_PAGE_META.title],
      ['meta[property="og:description"]', APP_PAGE_META.description],
      ['meta[property="og:image"]', `${window.location.origin}${APP_HERO_PREVIEW.src}`],
      ['meta[property="og:url"]', window.location.href],
    ]
    const previousMetadata = metadata.map(([selector, content]) => {
      const element = document.querySelector(selector)
      const previousContent = element?.getAttribute('content') ?? ''
      element?.setAttribute('content', content)
      return [element, previousContent]
    })
    document.title = APP_PAGE_META.title

    return () => {
      document.title = previousTitle
      previousMetadata.forEach(([element, content]) => element?.setAttribute('content', content))
    }
  }, [])

  const pauseBackgroundMusic = () => {
    window.dispatchEvent(new CustomEvent('site:pause-music'))
  }

  return (
    <div className="app-page">
      <section className="app-page__hero wrap" aria-labelledby="app-page-title">
        <div className="app-page__hero-copy">
          <p className="eyebrow">九型芯之力 App</p>
          <h1 className="display" id="app-page-title">
            把理解自己，<br /><span className="gradient-text">带进每一天</span>
          </h1>
          <p className="lead">从九型测评、智能对话到关系洞察与人生故事，把每一次觉察沉淀为看得见的成长轨迹。</p>
          <nav className="app-page__quick-links" aria-label="本页导航">
            <a href="#download-app">立即下载</a>
            <a href="#app-features">功能介绍</a>
            <a href="#install-guide">安装说明</a>
            <a href="#release-notes">更新记录</a>
          </nav>
        </div>
        <figure className="app-page__device">
          <div className="app-page__device-screen">
            <img
              src={APP_HERO_PREVIEW.src}
              alt={APP_HERO_PREVIEW.alt}
              width="448"
              height="960"
              fetchPriority="high"
            />
          </div>
        </figure>
      </section>

      <AppDownloadSection showInstallSummary={false} />

      <section className="wrap block app-page__features" id="app-features" aria-labelledby="app-features-title" tabIndex="-1">
        <Reveal className="section-head app-page__section-head">
          <p className="eyebrow">核心功能</p>
          <h2 className="section-title" id="app-features-title">一处记录，持续看见自己的变化</h2>
          <p className="lead">功能围绕认识自己、理解关系与长期成长展开，不需要在多个工具之间来回切换。</p>
        </Reveal>
        <div className="app-page__feature-grid">
          {APP_PAGE_FEATURES.map((feature) => (
            <Reveal className="app-page__feature" key={feature.title}>
              <span className="app-page__feature-icon"><FeatureIcon name={feature.icon} /></span>
              <h3>{feature.title}</h3>
              <p>{feature.description}</p>
            </Reveal>
          ))}
        </div>
      </section>

      <section className="app-page__install-band" id="install-guide" aria-labelledby="install-guide-title" tabIndex="-1">
        <div className="wrap">
          <Reveal className="section-head app-page__install-head">
            <p className="eyebrow">安装说明</p>
            <h2 className="section-title" id="install-guide-title">四步完成 Android 安装</h2>
            <p className="lead">先按步骤操作；需要对照手机界面时，再查看右侧的完整实机演示。</p>
          </Reveal>
          <div className="app-page__install-grid">
          <Reveal className="app-page__steps-column">
            <p className="eyebrow">安装步骤</p>
            <ol className="app-page__steps">
              {APP_INSTALL_STEPS.map((step) => (
                <li key={step.number}>
                  <span>{step.number}</span>
                  <div>
                    <h3>{step.title}</h3>
                    <p>{step.description}</p>
                  </div>
                </li>
              ))}
            </ol>
            <div className="app-page__security-note">
              <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor"
                   strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M12 3 5 6v5c0 4.6 2.8 8.1 7 10 4.2-1.9 7-5.4 7-10V6l-7-3ZM9 12l2 2 4-5" />
              </svg>
              <p><strong>请认准官网安装包。</strong> 安装时 Android 可能提示“未知来源”，这是浏览器下载 APK 的系统提示。</p>
            </div>
          </Reveal>

            <Reveal className="app-page__video-column">
              <p className="eyebrow">安装视频</p>
              <h3 className="app-page__video-title">跟着视频完成安装</h3>
              <p className="app-page__video-lead">视频演示从官网下载 APK、处理系统安装提示，到打开 App 注册使用的完整过程。</p>
              <div className="app-page__video-meta">
                <span>{APP_INSTALL_VIDEO.duration}</span>
                <span>竖屏实机演示</span>
                <span>含中文字幕</span>
              </div>
              <div className="app-page__video-frame">
                <video
                  controls
                  playsInline
                  preload="metadata"
                  poster={APP_INSTALL_VIDEO.poster}
                  onPlay={pauseBackgroundMusic}
                >
                  <source src={APP_INSTALL_VIDEO.src} type="video/mp4" />
                  <track
                    kind="captions"
                    src={APP_INSTALL_VIDEO.captions}
                    srcLang="zh-CN"
                    label="简体中文"
                    default
                  />
                </video>
              </div>
              <p className="app-page__video-fallback">
                如果视频无法播放，<a href={APP_INSTALL_VIDEO.src}>打开安装视频文件</a>。
              </p>
            </Reveal>
          </div>
        </div>
      </section>

      <section className="wrap block app-page__releases" id="release-notes" aria-labelledby="release-notes-title" tabIndex="-1">
        <Reveal className="section-head app-page__section-head">
          <p className="eyebrow">更新记录</p>
          <h2 className="section-title" id="release-notes-title">每次更新，都让陪伴更稳一点</h2>
          <p className="lead">{APP_RELEASE_DISCLAIMER}</p>
        </Reveal>
        <div className="app-page__release-list">
          {APP_RELEASE_HISTORY.map((release) => (
            <Reveal className="app-page__release-item" key={release.version}>
              <div className="app-page__release-mark" aria-hidden="true"><span /></div>
              <div className="app-page__release-version">
                <strong>v{release.version}</strong>
                <time dateTime={release.date}>{release.date}</time>
              </div>
              <div className="app-page__release-copy">
                <h3>{release.title}</h3>
                <ul>{release.notes.map((note) => <li key={note}>{note}</li>)}</ul>
              </div>
            </Reveal>
          ))}
        </div>
      </section>

      <section className="wrap app-page__closing">
        <img src={siteConfig.site.logo} alt="" width="56" height="56" />
        <div>
          <h2>从今天的一次觉察开始</h2>
          <p>下载 Android 正式版，把成长记录在离你最近的地方。</p>
        </div>
        <a className="btn btn--red" href="#download-app">前往下载</a>
      </section>
    </div>
  )
}
