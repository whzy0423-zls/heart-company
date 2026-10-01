// 小程序公共微信支付封装：业务只负责下单、开发模拟、查状态和成功刷新。
// 金额、商户参数与签名材料始终由服务端订单接口提供，前端不自行生成或覆盖。
import { request } from '../api/request'
import { createReportOrderApi, reportStatusApi } from '../api'
import { createWechatPaymentController as createOrderController } from './classroomProgress'

const REQUIRED_PAY_FIELDS = ['timeStamp', 'nonceStr', 'package', 'paySign']

function normalizePayParams(payParams) {
  if (!payParams || typeof payParams !== 'object') throw new Error('微信支付参数不完整')
  const missing = REQUIRED_PAY_FIELDS.filter((field) => !String(payParams[field] || '').trim())
  if (missing.length) throw new Error('微信支付参数不完整')
  return payParams
}

// 仅负责调起一次真实微信收银台；返回成功不等于服务端已完成落账。
export async function requestWechatPayment(payParams) {
  const pay = normalizePayParams(payParams)
  // Use uni-app's injected runtime so the WeChat compiler binds this API.
  // globalThis.uni is not guaranteed to exist in the mini-program sandbox.
  if (typeof uni === 'undefined' || typeof uni.requestPayment !== 'function') throw new Error('当前环境不支持微信支付')
  return new Promise((resolve, reject) => {
    uni.requestPayment({
      provider: 'wxpay',
      timeStamp: pay.timeStamp,
      nonceStr: pay.nonceStr,
      package: pay.package,
      signType: pay.signType || 'RSA',
      paySign: pay.paySign,
      success: resolve,
      fail: reject,
    })
  })
}

// 根据订单上的 devMode 选择开发模拟或真实支付，开发模拟接口由业务方注入。
export async function payWechatOrder(order, { devPay } = {}) {
  if (order?.payParams?.devMode === true) {
    if (typeof devPay !== 'function') throw new Error('开发模拟支付方法未配置')
    return devPay(order)
  }
  return requestWechatPayment(order?.payParams)
}

export function createWechatPaymentController(options = {}) {
  const { devPay, pay, ...controllerOptions } = options
  return createOrderController({
    ...controllerOptions,
    pay: typeof pay === 'function' ? pay : (order) => payWechatOrder(order, { devPay }),
  })
}

function paymentError(snapshot = {}) {
  const error = new Error(snapshot.message || '支付失败，请重试')
  if (snapshot.state === 'cancelled') error.errMsg = 'requestPayment:fail cancel'
  return error
}

// 报告页兼容入口：支付调起后必须等服务端把报告标记为 unlocked 才返回成功。
export async function payForReport(testRecordId) {
  const controller = createWechatPaymentController({
    create: () => createReportOrderApi(testRecordId),
    devPay: (order) =>
      request({
        url: '/miniapp/report/dev-pay',
        method: 'POST',
        auth: true,
        data: { out_trade_no: order.outTradeNo, trade_state: 'SUCCESS' },
      }),
    status: () => reportStatusApi(testRecordId),
    isPaid: (status) => status?.unlocked === true,
  })
  const result = await controller.purchase()
  if (result?.state !== 'success') throw paymentError(result)
  return {
    ok: true,
    dev: result.order?.payParams?.devMode === true,
    order: result.order,
    status: result.status,
  }
}
