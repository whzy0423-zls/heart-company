import { useEffect, useRef, useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { invitationCode, registerInvitedAccount, sendRegistrationCode, validateRegistration } from '../api/inviteRegistration'
import { buildLatestAppReleaseDownloadURL } from '../api/appRelease'
import './InviteRegister.css'

export default function InviteRegister() {
  const { search } = useLocation()
  const [form, setForm] = useState({ nickname: '', account: '', password: '', confirmPassword: '', phone: '', code: '', agentCode: invitationCode(search), consent: false })
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [sending, setSending] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [countdown, setCountdown] = useState(0)
  const [registered, setRegistered] = useState(null)
  const requestLock = useRef(false)
  const mounted = useRef(true)
  useEffect(() => { mounted.current = true; return () => { mounted.current = false } }, [])
  useEffect(() => { setForm(previous => ({ ...previous, agentCode: invitationCode(search) })) }, [search])
  useEffect(() => {
    if (!countdown) return
    const timer = window.setTimeout(() => setCountdown(value => Math.max(0, value - 1)), 1000)
    return () => window.clearTimeout(timer)
  }, [countdown])
  const update = (event) => {
    const { name, value, checked, type } = event.target
    setForm(previous => ({ ...previous, [name]: type === 'checkbox' ? checked : value }))
  }
  async function sendCode() {
    if (requestLock.current || countdown > 0) return
    setError(''); setMessage('')
    if (!form.consent) { setError('请先阅读并同意隐私政策，再获取验证码'); return }
    requestLock.current = true; setSending(true)
    try {
      await sendRegistrationCode(form.phone)
      if (mounted.current) { setCountdown(60); setMessage('验证码已发送，请查收短信。') }
    } catch (failure) {
      if (mounted.current) setError(failure.message)
    } finally {
      requestLock.current = false
      if (mounted.current) setSending(false)
    }
  }
  async function submit(event) {
    event.preventDefault()
    if (requestLock.current) return
    setError(''); setMessage('')
    const invalid = validateRegistration(form)
    if (invalid) { setError(invalid); return }
    requestLock.current = true; setSubmitting(true)
    try {
      const result = await registerInvitedAccount(form)
      if (mounted.current) {
        setRegistered(result)
        setForm(previous => ({ ...previous, password: '', confirmPassword: '', code: '' }))
      }
    } catch (failure) {
      if (mounted.current) setError(failure.message)
    } finally {
      requestLock.current = false
      if (mounted.current) setSubmitting(false)
    }
  }
  const busy = sending || submitting
  return (
    <section className="invite-register wrap" aria-labelledby="invite-title">
      <header>
        <p className="eyebrow">芯之力 · 邀请注册</p>
        <h1 id="invite-title">从认识自己开始</h1>
        <p>创建你的芯之力 App 账号，开启自己的成长旅程。</p>
      </header>
      {registered ? <div className="invite-register__card" role="status">
        <h2>注册成功</h2>
        <p>账号：<strong>{registered.account}</strong></p>
        <p>在 App 中使用该账号或手机号与刚设置的密码登录，无需重复注册。</p>
        <a className="invite-register__primary" href={buildLatestAppReleaseDownloadURL()}>下载最新版 Android App</a>
        <Link to="/app">查看下载与安装说明</Link>
      </div> : <form className="invite-register__card" onSubmit={submit}>
        <div className="invite-register__invitation">
          <label htmlFor="invite-agent-code">邀请码（选填）</label>
          <input id="invite-agent-code" name="agentCode" value={form.agentCode} onChange={update} maxLength={100} disabled={busy} autoComplete="off" placeholder="没有邀请码也可注册" />
          <p>注册时会关联此邀请码对应的邀请人。请确认后提交；邀请码是否有效以服务端校验为准。</p>
        </div>
        <fieldset disabled={busy}>
          <label htmlFor="invite-nickname">昵称</label>
          <input id="invite-nickname" name="nickname" value={form.nickname} onChange={update} required maxLength={32} autoComplete="nickname" placeholder="希望我们如何称呼你" />
          <label htmlFor="invite-account">登录账号</label>
          <input id="invite-account" name="account" value={form.account} onChange={update} required pattern="[A-Za-z][A-Za-z0-9_]{3,31}" maxLength={32} autoComplete="username" autoCapitalize="none" aria-describedby="invite-account-hint" />
          <small id="invite-account-hint">字母开头，4–32 位字母、数字或下划线</small>
          <label htmlFor="invite-password">设置密码</label>
          <input id="invite-password" name="password" type="password" value={form.password} onChange={update} required maxLength={72} autoComplete="new-password" aria-describedby="invite-password-hint" />
          <small id="invite-password-hint">6–72 字节，建议使用字母、数字和符号组合</small>
          <label htmlFor="invite-confirm">确认密码</label>
          <input id="invite-confirm" name="confirmPassword" type="password" value={form.confirmPassword} onChange={update} required maxLength={72} autoComplete="new-password" />
          <label htmlFor="invite-phone">中国大陆手机号</label>
          <input id="invite-phone" name="phone" type="tel" inputMode="tel" value={form.phone} onChange={update} required pattern="1[3-9][0-9]{9}" maxLength={11} autoComplete="tel-national" />
          <label htmlFor="invite-code">短信验证码</label>
          <div className="invite-register__code">
            <input id="invite-code" name="code" inputMode="numeric" value={form.code} onChange={update} required pattern="[0-9]{6}" maxLength={6} autoComplete="one-time-code" />
            <button type="button" onClick={sendCode} disabled={busy || countdown > 0}>{sending ? '正在发送…' : countdown > 0 ? `${countdown} 秒后重发` : '获取验证码'}</button>
          </div>
          <label className="invite-register__consent">
            <input name="consent" type="checkbox" checked={form.consent} onChange={update} required />
            <span>我已阅读并同意<a href="/legal/privacy-policy.html" target="_blank" rel="noreferrer">《隐私政策》</a>，了解手机号用于注册验证，邀请码用于建立邀请关系。</span>
          </label>
        </fieldset>
        {error && <p className="invite-register__error" role="alert">{error}</p>}
        {message && <p role="status">{message}</p>}
        <button className="invite-register__primary" type="submit" disabled={busy}>{submitting ? '正在注册…' : '创建 App 账号'}</button>
        <p className="invite-register__footnote">已有账号？<Link to="/app">下载并打开 App 登录</Link>。已注册账号不会因再次打开此链接而更换邀请人。</p>
      </form>}
    </section>
  )
}
