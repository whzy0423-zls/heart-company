const API_BASE = import.meta.env?.VITE_API_BASE_URL || '/api'

export function invitationCode(search = '') {
  const params = new URLSearchParams(search)
  return (params.get('agentCode') || params.get('agent') || '').trim()
}

export function validateRegistration(values) {
  if (!values.consent) return '请先阅读并同意隐私政策'
  if (!values.nickname.trim() || [...values.nickname.trim()].length > 32) return '昵称需为 1–32 个字符'
  if (!/^[A-Za-z][A-Za-z0-9_]{3,31}$/.test(values.account.trim())) return '账号需为字母开头的 4–32 位字母、数字或下划线'
  const passwordBytes = new TextEncoder().encode(values.password).length
  if (passwordBytes < 6 || passwordBytes > 72) return '密码需为 6–72 字节（中文字符通常占 3 字节）'
  if (values.password !== values.confirmPassword) return '两次输入的密码不一致'
  if (!/^1[3-9]\d{9}$/.test(values.phone.trim())) return '请输入正确的中国大陆手机号'
  if (!/^\d{6}$/.test(values.code.trim())) return '请输入 6 位短信验证码'
  return ''
}

async function postAuth(path, payload, { fetchImpl = globalThis.fetch, apiBase = API_BASE } = {}) {
  let response
  try {
    response = await fetchImpl(`${apiBase.replace(/\/+$/, '')}/app/auth/${path}`, {
      method: 'POST',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
      signal: AbortSignal.timeout(30000),
    })
  } catch {
    throw new Error('网络连接中断，请稍后重试；若已提交注册，可先在 App 尝试登录。')
  }
  const body = await response.json().catch(() => null)
  if (!response.ok || body?.code !== 0) {
    throw new Error(body?.error || body?.message || '请求未完成，请稍后重试')
  }
  return body.data
}

export function sendRegistrationCode(phone, options) {
  const normalized = phone.trim()
  if (!/^1[3-9]\d{9}$/.test(normalized)) return Promise.reject(new Error('请输入正确的中国大陆手机号'))
  return postAuth('send-sms', { phone: normalized, purpose: 'register' }, options)
}

export async function registerInvitedAccount(values, options) {
  const error = validateRegistration(values)
  if (error) throw new Error(error)
  const data = await postAuth('register', {
    nickname: values.nickname.trim(), account: values.account.trim(),
    password: values.password, phone: values.phone.trim(), code: values.code.trim(),
    ...(values.agentCode.trim() ? { agentCode: values.agentCode.trim() } : {}),
    deviceInfo: 'website-invite',
  }, options)
  if (!data?.user?.id) throw new Error('注册结果未确认，请先在 App 尝试登录，避免重复注册。')
  // The website does not start a signed-in session or persist returned credentials.
  return { account: values.account.trim() }
}
