import { lazy, Suspense } from 'react'
import { Link } from 'react-router-dom'
import AppDownloadSection from '../components/AppDownloadSection'
import Reveal from '../components/Reveal'
import MotionBackdrop from '../components/MotionBackdrop'
import MotionStory from '../components/MotionStory'
import EnneagramField from '../motion/EnneagramField'
import { QUESTIONS } from '../data/enneagramGame'
import siteConfig from '../data/siteConfig'
import { FEATURED_VIDEOS } from '../data/videos'
import { QuotesSection, SignupSection, TypesSection } from './homeSections'

const EnneagramScene = lazy(() => import('../components/EnneagramScene'))

export default function Home() {
  const { home } = siteConfig

  return (
    <div className="motion-home">
      <MotionBackdrop />
      {/* Hero */}
      <section className="motion-hero wrap motion-reveal" data-motion-section="hero" data-motion-reveal>
        <div className="motion-hero__matrix" aria-hidden="true">
          {Array.from({ length: 36 }, (_, index) => <span key={index}>芯 之 力</span>)}
        </div>
        <EnneagramField />
        <Suspense fallback={null}><EnneagramScene /></Suspense>
        <div className="motion-hero__grid">
          <div>
            <p className="motion-kicker">{home.hero.eyebrow} · INNER POWER</p>
            <h1 className="motion-display" data-motion-title><span className="motion-word">你好，</span><br /><span className="motion-word">这里是</span><br /><span className="motion-word"><em>芯之力</em></span></h1>
            <p className="motion-lead">{home.hero.lead} 从九型人格开始，辨认自己的能量模式，也理解每一段关系里的真实需要。</p>
            <div className="btn-row motion-hero__actions">
              {home.hero.actions.map((action) => (
                action.type === 'anchor'
                  ? <a key={action.label} className={`btn btn--${action.variant} motion-btn`} href={action.to} data-magnetic>{action.label}</a>
                  : <Link key={action.label} className={`btn btn--${action.variant} motion-btn`} to={action.to} data-magnetic>{action.label}</Link>
              ))}
            </div>
            <div className="motion-note"><strong>09</strong><span>种性格模式 · 一条完整的自我理解路径</span></div>
            <div className="stats motion-stats">
              {home.hero.stats.map((stat) => (
                <div className="stat" key={stat.label}>
                  <b data-count={stat.value} data-suffix={stat.suffix || undefined}>0</b>
                  <span>{stat.label}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="motion-hero__scene-space" aria-hidden="true" />
        </div>
        <div className="motion-hero__rail" aria-label="核心业务入口">
          <Link to="/watch"><span>01</span><strong>开始观看</strong><i aria-hidden="true">↗</i></Link>
          <Link to="/stages"><span>02</span><strong>成长三阶段</strong><i aria-hidden="true">↗</i></Link>
          <a href="#enterprise"><span>03</span><strong>企业与关系成长</strong><i aria-hidden="true">↓</i></a>
        </div>
      </section>

      <AppDownloadSection />

      {/* 老师简介 teaser */}
      <section className="wrap block motion-section motion-reveal" id="teacher" data-motion-section="teacher" data-motion-reveal>
        <Reveal className="panel split split--a">
          <img src={home.teacherTeaser.image} alt={home.teacherTeaser.title} data-parallax
               onError={(e) => { e.currentTarget.onerror = null; e.currentTarget.src = home.teacherTeaser.fallbackImage }}
               style={{ borderRadius: 14, boxShadow: 'var(--shadow)', width: '100%' }} />
          <div>
            <p className="eyebrow">{home.teacherTeaser.eyebrow}</p>
            <h2 className="section-title">{home.teacherTeaser.title}</h2>
            <p className="lead" style={{ margin: '14px 0 22px' }}>{home.teacherTeaser.lead}</p>
            <Link className="btn btn--blue" to={home.teacherTeaser.buttonTo}>{home.teacherTeaser.buttonText}</Link>
          </div>
        </Reveal>
      </section>

      <MotionStory />

      {/* 课程 */}
      <section className="wrap block motion-section motion-reveal" id="courses" data-motion-section="courses" data-motion-reveal>
        <Reveal className="section-head">
          <p className="eyebrow">{home.courses.eyebrow}</p>
          <h2 className="section-title">{home.courses.title}</h2>
        </Reveal>
        <div className="grid grid-3">
          {home.courses.items.map((course) => (
            <Reveal className="card course-card" key={course.badge}>
              <div className="card-head">
                <span className="badge">{course.badge}</span>
                <h3>{course.title}</h3>
              </div>
              <p style={{ color: 'var(--muted)', fontSize: '14.5px' }}>{course.description}</p>
              <ul className="bullets" style={{ marginTop: 14, fontSize: 14 }}>
                {course.bullets.map((item) => <li key={item}>{item}</li>)}
              </ul>
            </Reveal>
          ))}
        </div>
      </section>

      {/* 视频精选 */}
      <section className="wrap block motion-section motion-section--dark motion-reveal" id="game" data-motion-section="video-game" data-motion-reveal>
        <Reveal className="panel home-video" style={{ overflow: 'visible' }}>
          <div className="home-video__head">
            <div>
              <p className="eyebrow">{home.game.eyebrow}</p>
              <h2 className="section-title">先看精选视频</h2>
              <p className="lead" style={{ margin: '14px 0 22px' }}>{home.game.lead}</p>
            </div>
            <Link className="home-video__more" to="/watch">更多视频</Link>
          </div>
          <div className="home-video__grid">
            {FEATURED_VIDEOS.map((video) => (
              <Link className="home-video-card" key={video.id} to="/watch">
                <span className="home-video-card__media">
                  <img src={video.poster} alt={video.title} loading="lazy" />
                  <i aria-hidden="true">▶</i>
                </span>
                <div className="home-video-card__body">
                  <span>{video.tag} · {video.duration}</span>
                  <h3>{video.title}</h3>
                </div>
              </Link>
            ))}
          </div>
          <div className="home-video__actions">
            <Link className="btn btn--red" to="/game">进入小游戏体验 →</Link>
            <span>约 2 分钟 · {QUESTIONS.length} 题</span>
          </div>
        </Reveal>
      </section>

      {/* 三阶段 */}
      <section className="wrap block motion-section motion-reveal" id="stages" data-motion-section="stages" data-motion-reveal>
        <Reveal className="section-head">
          <p className="eyebrow">{home.stages.eyebrow}</p>
          <h2 className="section-title">{home.stages.title}</h2>
          <p className="lead" style={{ marginTop: 12 }}>{home.stages.lead}</p>
        </Reveal>
        <div className="grid grid-3">
          {home.stages.items.map((stage) => (
            <Reveal as={Link} to={stage.to} className="card" key={stage.to}>
              <p className="kicker" style={{ color: 'var(--red)', fontSize: 13 }}>{stage.kicker}</p>
              <h3 style={{ margin: '8px 0' }}>{stage.title}</h3>
              <p style={{ color: 'var(--blue)', fontWeight: 700, fontSize: 14 }}>{stage.subtitle}</p>
              <p style={{ color: 'var(--muted)', fontSize: 14, marginTop: 10 }}>{stage.description}</p>
              <p style={{ color: 'var(--blue)', fontWeight: 700, marginTop: 14 }}>进入单独介绍 →</p>
            </Reveal>
          ))}
        </div>
        <div className="btn-row" style={{ justifyContent: 'center', marginTop: 28 }}>
          <Link className="btn btn--blue" to="/course">查看完整课件 →</Link>
        </div>
      </section>

      {/* 企业 */}
      <section className="wrap block motion-section motion-reveal" id="enterprise" data-motion-section="enterprise" data-motion-reveal>
        <Reveal className="panel split split--b">
          <div>
            <p className="eyebrow">{home.enterprise.eyebrow}</p>
            <h2 className="section-title">{home.enterprise.title}</h2>
            <p className="lead" style={{ margin: '14px 0 22px' }}>{home.enterprise.lead}</p>
            <a className="btn btn--red" href={home.enterprise.buttonHref}>{home.enterprise.buttonText}</a>
          </div>
          <div className="card" style={{ alignSelf: 'start' }}>
            <h4 style={{ marginBottom: 12 }}>{home.enterprise.moduleTitle}</h4>
            <ul className="bullets" style={{ fontSize: 14 }}>
              {home.enterprise.modules.map((item) => <li key={item}>{item}</li>)}
            </ul>
          </div>
        </Reveal>
      </section>

      <QuotesSection />
      <TypesSection />
      <SignupSection />
    </div>
  )
}
