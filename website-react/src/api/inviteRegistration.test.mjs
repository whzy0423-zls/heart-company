import test from 'node:test'
import assert from 'node:assert/strict'
import { invitationCode, validateRegistration, sendRegistrationCode, registerInvitedAccount } from './inviteRegistration.js'
const valid = { nickname: '小林', account: 'user_123', password: 'secret123', confirmPassword: 'secret123', phone: '13800000000', code: '123456', agentCode: ' A1-2 ', consent: true }
const success = (data) => ({ ok: true, json: async () => ({ code: 0, data }) })
test('both current and legacy invitation links retain code', () => {
  assert.equal(invitationCode('?agentCode=A1-2'), 'A1-2')
  assert.equal(invitationCode('?agent=A1%2B2'), 'A1+2')
  assert.equal(invitationCode('?agentCode=A1&agent=A2'), 'A1')
  assert.equal(invitationCode(''), '')
})
test('validation rejects invalid form before sending', async () => {
  for (const change of [{ consent: false }, { nickname: '' }, { account: '12ab' }, { password: '短' }, { confirmPassword: 'mismatch' }, { phone: '111' }, { code: '123' }]) {
    assert.ok(validateRegistration({ ...valid, ...change }))
    await assert.rejects(registerInvitedAccount({ ...valid, ...change }, { fetchImpl: () => { throw new Error('should not fetch') } }))
  }
})
test('SMS uses actual register purpose', async () => {
  await sendRegistrationCode(valid.phone, { fetchImpl: async (url, request) => {
    assert.equal(url, '/api/app/auth/send-sms')
    assert.deepEqual(JSON.parse(request.body), { phone: valid.phone, purpose: 'register' })
    return success(null)
  } })
})
test('registration submits invitation and only returns public success state', async () => {
  const result = await registerInvitedAccount(valid, { fetchImpl: async (url, request) => {
    assert.equal(url, '/api/app/auth/register')
    assert.deepEqual(JSON.parse(request.body), { nickname: valid.nickname, account: valid.account, password: valid.password, phone: valid.phone, code: valid.code, agentCode: 'A1-2', deviceInfo: 'website-invite' })
    return success({ user: { id: 1 }, accessToken: 'not-retained', refreshToken: 'not-retained' })
  } })
  assert.deepEqual(result, { account: valid.account })
})
test('HTTP, application, and malformed success errors never look successful', async () => {
  for (const response of [
    { ok: false, json: async () => ({ error: '邀请码不存在或已失效' }) },
    { ok: true, json: async () => ({ code: 1, message: '验证码错误' }) },
    success(null),
    { ok: true, json: async () => { throw new Error('HTML fallback') } },
  ]) await assert.rejects(registerInvitedAccount(valid, { fetchImpl: async () => response }))
})
