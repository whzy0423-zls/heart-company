import { useState } from 'react'
import { Link } from 'react-router-dom'
import Reveal from '../components/Reveal'
import Wheel from '../components/Wheel'
import { QUESTIONS } from '../data/enneagramGame'
import { TYPES } from '../data/types'
import siteConfig from '../data/siteConfig'
import { submitSignup } from '../api/signup'

export default function Home() {
  const [submitted, setSubmitted] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState('')
  const [contactType, setContactType] = useState('phone')
  const { home } = siteConfig
  const phonePattern = /^1[3-9]\d{9}$/

  async function handleSignupSubmit(e) {
    e.preventDefault()
    const form = e.currentTarget
    const data = new FormData(form)
    const name = String(data.get('name') || '').trim()
    const selectedContactType = String(data.get('contactType') || 'phone')
    const rawContact = String(data.get('contact') || '').trim()
    const contact = selectedContactType === 'phone'
      ? rawContact.replace(/[\s\-－]/g, '')
      : rawContact
    if (!name) {
      setSubmitted(false)
      setSubmitError('请输入你的称呼')
      return
    }
    if (!contact) {
      setSubmitted(false)
      setSubmitError(selectedContactType === 'phone' ? '请输入手机号' : '请输入微信号')
      return
    }
    if (selectedContactType === 'phone' && !phonePattern.test(contact)) {
      setSubmitted(false)
      setSubmitError('请输入正确的手机号')
      return
    }
    setSubmitting(true)
    setSubmitError('')
    setSubmitted(false)
    try {
      await submitSignup({
        contact,
        contactType: selectedContactType,
        interest: String(data.get('interest') || ''),
        message: String(data.get('message') || ''),
        name,
      })
      form.reset()
      setSubmitted(true)
    } catch (error) {
      setSubmitError(error?.message || '提交失败，请稍后再试')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      {/* Hero */}
      <section className="hero wrap">
        <div className="hero__grid">
          <div>
            <p className="eyebrow">{home.hero.eyebrow}</p>
            <h1 className="display"><span className="gradient-text">{home.hero.title}</span></h1>
            <p className="lead" style={{ marginTop: 18 }}>{home.hero.lead}</p>
            <div className="btn-row">
              {home.hero.actions.map((action) => (
                action.type === 'anchor'
                  ? <a key={action.label} className={`btn btn--${action.variant}`} href={action.to}>{action.label}</a>
                  : <Link key={action.label} className={`btn btn--${action.variant}`} to={action.to}>{action.label}</Link>
              ))}
            </div>
            <div className="stats">
              {home.hero.stats.map((stat) => (
                <div className="stat" key={stat.label}>
                  <b data-count={stat.value} data-suffix={stat.suffix || undefined}>0</b>
                  <span>{stat.label}</span>
                </div>
              ))}
            </div>
          </div>
          <Wheel />
        </div>
      </section>

      {/* 老师简介 teaser */}
      <section className="wrap block" id="teacher">
        <Reveal className="panel split split--a">
          <img src={home.teacherTeaser.image} alt={home.teacherTeaser.title}
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

      {/* 课程 */}
      <section className="wrap block" id="courses">
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

      {/* 小游戏 */}
      <section className="wrap block" id="game">
        <Reveal className="panel split" style={{ overflow: 'visible' }}>
          <div>
            <p className="eyebrow">{home.game.eyebrow}</p>
            <h2 className="section-title">{home.game.title}</h2>
            <p className="lead" style={{ margin: '14px 0 22px' }}>{home.game.lead}</p>
            <Link className="btn btn--red" to="/game">进入小游戏体验 →</Link>
          </div>
          <Link to="/game" className="figure game-entry" style={{ background: 'linear-gradient(150deg,#10243f,#0b1220)', aspectRatio: '16/10', display: 'grid', placeItems: 'center', position: 'relative', overflow: 'hidden', textDecoration: 'none' }}>
            <div style={{ position: 'absolute', width: 200, height: 200, borderRadius: '50%', background: 'radial-gradient(circle,rgba(43,127,255,.5),transparent 65%)', filter: 'blur(20px)' }}></div>
            <div style={{ position: 'relative', textAlign: 'center', color: '#cfe0ff' }}>
              <div style={{ width: 78, height: 78, borderRadius: '50%', background: 'var(--grad-blue)', display: 'grid', placeItems: 'center', margin: '0 auto 14px', boxShadow: 'var(--glow-blue)', fontSize: 30, color: '#fff' }}>▶</div>
              <div style={{ fontWeight: 700 }}>测一测你的性格芯片</div>
              <div style={{ fontSize: 13, opacity: .7, marginTop: 4 }}>约 2 分钟 · {QUESTIONS.length} 题</div>
            </div>
          </Link>
        </Reveal>
      </section>

      {/* 三阶段 */}
      <section className="wrap block" id="stages">
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
          <Link className="btn btn--blue" to="/course">📖 查看完整课件 →</Link>
        </div>
      </section>

      {/* 企业 */}
      <section className="wrap block" id="enterprise">
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

      {/* 老韩语录 */}
      <section className="wrap block" id="quotes">
        <Reveal className="section-head">
          <p className="eyebrow">{home.quotes.eyebrow}</p>
          <h2 className="section-title">{home.quotes.title}</h2>
          <p className="lead" style={{ marginTop: 12 }}>{home.quotes.lead}</p>
        </Reveal>
        <div className="grid grid-3">
          {home.quotes.items.map((quote) => (
            <Reveal as="blockquote" className="card" key={quote}>“{quote}”</Reveal>
          ))}
        </div>
      </section>

      {/* 九型概览 */}
      <section className="wrap block" id="types">
        <Reveal className="section-head">
          <p className="eyebrow">{home.typesSection.eyebrow}</p>
          <h2 className="section-title">{home.typesSection.title}</h2>
          <p className="lead" style={{ marginTop: 12 }}>{home.typesSection.lead}</p>
        </Reveal>
        <div className="grid grid-3">
          {TYPES.map((t) => (
            <Reveal key={t[0]} className="card type-card">
              <div className="type-card__head">
                <span className="type-card__avatar-wrap">
                  <img className="type-card__avatar" src={`/assets/avatars/${t[0]}.png`} alt={t[1]} loading="lazy"
                       onError={(e) => { e.currentTarget.style.display = 'none' }} />
                  <span className="type-card__num">{t[0]}</span>
                </span>
                <h3 className="type-card__name">{t[1]}</h3>
              </div>
              <p style={{ color: 'var(--blue)', fontWeight: 700, fontSize: 13, margin: '10px 0 6px' }}>{t[2]}</p>
              <p style={{ color: 'var(--muted)', fontSize: 14 }}>{t[3]}</p>
            </Reveal>
          ))}
        </div>
      </section>

      {/* 注册 / 互动 */}
      <section className="wrap block" id="signup">
        <Reveal className="panel split">
          <div>
            <p className="eyebrow">{home.signup.eyebrow}</p>
            <h2 className="section-title">{home.signup.title}</h2>
            <p className="lead" style={{ marginTop: 14 }}>{home.signup.lead}</p>
            <ul className="bullets" style={{ marginTop: 18, fontSize: 14 }}>
              {home.signup.bullets.map((item) => <li key={item}>{item}</li>)}
            </ul>
          </div>
          <form className="card form" style={{ display: 'grid', gap: 14 }}
                onSubmit={handleSignupSubmit} noValidate>
            <div className="form-row">
              <input className="field" name="name" required placeholder="你的称呼" />
              <select
                className="field field--select"
                name="contactType"
                value={contactType}
                onChange={(e) => {
                  setContactType(e.target.value)
                  setSubmitError('')
                }}>
                <option value="phone">手机号</option>
                <option value="wechat">微信号</option>
              </select>
            </div>
            <input
              className="field"
              name="contact"
              required
              inputMode={contactType === 'phone' ? 'tel' : 'text'}
              placeholder={contactType === 'phone' ? '请输入手机号' : '请输入微信号'} />
            <select className="field field--select" name="interest">
              {home.signup.interestOptions.map((item) => <option key={item} value={item}>{item}</option>)}
            </select>
            <textarea className="field" name="message" rows="3" placeholder="想咨询的问题或需求（选填）" style={{ resize: 'vertical' }}></textarea>
            <button className="btn btn--red" disabled={submitting} style={{ justifyContent: 'center' }}>
              {submitting ? '提交中...' : '提交咨询'}
            </button>
            <p className="ok" style={{ display: submitted ? 'block' : 'none', color: 'var(--blue)', fontWeight: 700, textAlign: 'center' }}>{home.signup.successText}</p>
            <p className="ok" style={{ display: submitError ? 'block' : 'none', color: 'var(--red)', fontWeight: 700, textAlign: 'center' }}>{submitError}</p>
          </form>
        </Reveal>
      </section>
    </>
  )
}
