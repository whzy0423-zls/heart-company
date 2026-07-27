import { getToken, request } from './request'
import { APP_CHANNEL } from '../config'
import { userErrorMessage } from '../utils/userMessage'

function classroomAuth() {
  return Boolean(getToken())
}

function classroomID(value) {
  const normalized = String(value ?? '').trim()
  if (!/^\d+$/.test(normalized) || normalized === '0') throw new Error('课件参数无效')
  return normalized
}

async function classroomRequest(options, fallback) {
  try {
    return await request(options)
  } catch (error) {
    const normalized = new Error(userErrorMessage(error, fallback))
    if (error && typeof error === 'object') Object.assign(normalized, error)
    normalized.message = userErrorMessage(error, fallback)
    throw normalized
  }
}

// App 手机号验证码：BaseURL 已包含 /api，接口路径只写 /app/xxx。
export function sendAppSmsApi(phone) {
  return request({
    url: '/app/auth/sms/send',
    method: 'POST',
    data: { phone },
  })
}

export function loginByAppSmsApi(phone, code, deviceInfo = '') {
  return request({
    url: '/app/auth/sms/login',
    method: 'POST',
    data: { phone, code, deviceInfo },
  })
}

export function registerAppPushApi(data) {
  return request({
    url: '/app/push/register',
    method: 'POST',
    data,
    auth: true,
  })
}

export function unregisterAppPushApi(registrationId) {
  return request({
    url: '/app/push/unregister',
    method: 'POST',
    data: { registrationId },
    auth: true,
  })
}

// 微信登录：用 wx.login 的 code 换取后端 token
export function wxLoginApi(code, scene = '') {
  return request({
    url: '/wx/login',
    method: 'POST',
    data: { code, channel: APP_CHANNEL, scene },
  })
}

export function getUserInfoApi() {
  return request({ url: '/wx/userinfo', method: 'GET', auth: true })
}

export function updateUserInfoApi(data) {
  return request({ url: '/wx/userinfo', method: 'PUT', data, auth: true })
}

// 测试存档
export function saveTestRecordApi(data) {
  return request({ url: '/miniapp/test-records', method: 'POST', data, auth: true })
}

export function listTestRecordsApi() {
  return request({ url: '/miniapp/test-records', method: 'GET', auth: true })
}

// 预约（同时落后台客户线索）
export function createBookingApi(data) {
  return request({ url: '/miniapp/bookings', method: 'POST', data, auth: true })
}

export function listBookingsApi() {
  return request({ url: '/miniapp/bookings', method: 'GET', auth: true })
}

// 站点内容（公开）
export function getSiteConfigApi() {
  return request({ url: '/public/site-config', method: 'GET' })
}

// 测试统计上报（公开，匿名也可）
export function reportGameResultApi(data) {
  return request({ url: '/public/game-results', method: 'POST', data })
}

// 深度报告：查询解锁状态
export function reportStatusApi(testRecordId) {
  return request({ url: '/miniapp/report/status', method: 'GET', query: { testRecordId }, auth: true })
}

// 深度报告：下单（返回小程序拉起支付参数）
export function createReportOrderApi(testRecordId) {
  return request({ url: '/miniapp/report/order', method: 'POST', data: { testRecordId }, auth: true })
}

// 深度报告：解锁后获取正文（LLM 生成，耗时较长）
export function reportContentApi(testRecordId) {
  return request({ url: '/miniapp/report/content', method: 'GET', query: { testRecordId }, auth: true, timeout: 30000 })
}

export function listClassroomSeriesApi(query = {}) {
  return classroomRequest({ url: '/public/classroom/series', method: 'GET', query, auth: classroomAuth() }, '系列课程加载失败，请重试')
}

export function listClassroomStandaloneApi(query = {}) {
  return classroomRequest({ url: '/public/classroom/standalone', method: 'GET', query, auth: classroomAuth() }, '课件列表加载失败，请重试')
}

export function getClassroomSeriesApi(id) {
  return classroomRequest({ url: `/public/classroom/series/${classroomID(id)}`, method: 'GET', auth: classroomAuth() }, '系列课程加载失败，请重试')
}

export function getClassroomContentApi(id) {
  return classroomRequest({ url: `/public/classroom/content/${classroomID(id)}`, method: 'GET', auth: classroomAuth() }, '课件详情加载失败，请重试')
}

export async function getClassroomPlaybackApi(id) {
  const contentId = classroomID(id)
  const auth = classroomAuth()
  let ticket = ''
  if (!auth) {
    const result = await classroomRequest({ url: `/public/classroom/content/${contentId}/ticket`, method: 'POST', data: {} }, '播放凭证获取失败，请重试')
    ticket = String(result?.ticket || '')
  }
  return classroomRequest({
    url: `/miniapp/classroom/content/${contentId}/play`,
    method: 'POST',
    data: ticket ? { ticket } : {},
    auth,
  }, '播放地址获取失败，请重试')
}

function isExpiredPlaybackError(error) {
  const code = String(error?.code || error?.errCode || '').toLowerCase()
  const message = String(error?.message || error?.errMsg || '').toLowerCase()
  return code.includes('expired') || message.includes('expiredtoken') || message.includes('url expired')
}

export async function withClassroomPlaybackRetry(contentId, consume) {
  let playback = await getClassroomPlaybackApi(contentId)
  try {
    return await consume(playback)
  } catch (error) {
    if (!isExpiredPlaybackError(error)) throw error
    playback = await getClassroomPlaybackApi(contentId)
    return consume(playback)
  }
}

export function createClassroomOrderApi(targetType, refId) {
  return classroomRequest({ url: '/miniapp/classroom/orders', method: 'POST', data: { targetType, refId: classroomID(refId) }, auth: true }, '课堂订单创建失败，请重试')
}

export function getClassroomOrderStatusApi(targetType, refId) {
  return classroomRequest({ url: '/miniapp/classroom/orders/status', method: 'GET', query: { targetType, refId: classroomID(refId) }, auth: true }, '订单状态查询失败，请重试')
}

export function updateClassroomProgressApi(contentId, positionSeconds) {
  return classroomRequest({
    url: `/miniapp/classroom/content/${classroomID(contentId)}/progress`,
    method: 'PUT',
    data: { positionSeconds: Math.max(0, Math.floor(Number(positionSeconds) || 0)) },
    auth: true,
  }, '学习进度同步失败，请重试')
}

export function getClassroomContinueLearningApi() {
  return classroomRequest({ url: '/miniapp/classroom/continue-learning', method: 'GET', auth: true }, '继续学习记录加载失败，请重试')
}
