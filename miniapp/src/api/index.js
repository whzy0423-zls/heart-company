import { request } from './request'
import { APP_CHANNEL } from '../config'

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

// 九型 AI 对话（RAG 检索）
export function chatApi(data) {
  return request({ url: '/miniapp/chat', method: 'POST', data, auth: true, timeout: 30000 })
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
  return request({ url: `/miniapp/report/status?testRecordId=${testRecordId}`, method: 'GET', auth: true })
}

// 深度报告：下单（返回小程序拉起支付参数）
export function createReportOrderApi(testRecordId) {
  return request({ url: '/miniapp/report/order', method: 'POST', data: { testRecordId }, auth: true })
}

// 深度报告：解锁后获取正文（LLM 生成，耗时较长）
export function reportContentApi(testRecordId) {
  return request({ url: `/miniapp/report/content?testRecordId=${testRecordId}`, method: 'GET', auth: true, timeout: 30000 })
}
