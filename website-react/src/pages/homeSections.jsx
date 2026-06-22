import { useState } from 'react'
import { Link } from 'react-router-dom'
import Reveal from '../components/Reveal'
import { TYPES } from '../data/types'
import siteConfig from '../data/siteConfig'
import { submitSignup } from '../api/signup'

export function QuotesSection() {
  const { quotes } = siteConfig.home

  return (
    <section className="wrap block" id="quotes">
      <Reveal className="section-head">
        <p className="eyebrow">{quotes.eyebrow}</p>
        <h2 className="section-title">{quotes.title}</h2>
        <p className="lead" style={{ marginTop: 12 }}>{quotes.lead}</p>
      </Reveal>
      <div className="grid grid-3">
        {quotes.items.map((quote) => (
          <Reveal as="blockquote" className="quote-card card" key={quote}>“{quote}”</Reveal>
        ))}
      </div>
    </section>
  )
}

export function TypesSection() {
  const { typesSection } = siteConfig.home

  return (
    <section className="wrap block" id="types">
      <Reveal className="section-head">
        <p className="eyebrow">{typesSection.eyebrow}</p>
        <h2 className="section-title">{typesSection.title}</h2>
        <p className="lead" style={{ marginTop: 12 }}>{typesSection.lead}</p>
      </Reveal>
      <div className="grid grid-3">
        {TYPES.map((t) => (
          <Reveal as={Link} to={`/type/${t[0]}`} key={t[0]} className="card type-card type-card--link">
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
            <p className="type-card__more">查看型号详情 →</p>
          </Reveal>
        ))}
      </div>
    </section>
  )
}

export function SignupSection() {
  const [submitted, setSubmitted] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState('')
  const [contactType, setContactType] = useState('phone')
  const { signup } = siteConfig.home
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
    <section className="wrap block" id="signup">
      <Reveal className="panel split">
        <div>
          <p className="eyebrow">{signup.eyebrow}</p>
          <h2 className="section-title">{signup.title}</h2>
          <p className="lead" style={{ marginTop: 14 }}>{signup.lead}</p>
          <ul className="bullets" style={{ marginTop: 18, fontSize: 14 }}>
            {signup.bullets.map((item) => <li key={item}>{item}</li>)}
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
            {signup.interestOptions.map((item) => <option key={item} value={item}>{item}</option>)}
          </select>
          <textarea className="field" name="message" rows="3" placeholder="想咨询的问题或需求（选填）" style={{ resize: 'vertical' }}></textarea>
          <button className="btn btn--red" disabled={submitting} style={{ justifyContent: 'center' }}>
            {submitting ? '提交中...' : '提交咨询'}
          </button>
          <p className="ok" style={{ display: submitted ? 'block' : 'none', color: 'var(--blue)', fontWeight: 700, textAlign: 'center' }}>{signup.successText}</p>
          <p className="ok" style={{ display: submitError ? 'block' : 'none', color: 'var(--red)', fontWeight: 700, textAlign: 'center' }}>{submitError}</p>
        </form>
      </Reveal>
    </section>
  )
}
