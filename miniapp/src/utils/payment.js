// 微信支付封装：下单拿到 payParams 后拉起 uni.requestPayment。
// dev 模式（payParams.devMode=true，后端未配真实商户号）下无法真正拉起，
// 改走 app 鉴权的本地模拟接口，避免复用公开微信回调地址。
import { request } from '../api/request'
import { createReportOrderApi } from '../api'

// 调起一次报告解锁支付。成功 resolve，失败/取消 reject。
export async function payForReport(testRecordId) {
  const order = await createReportOrderApi(testRecordId)
  const pay = order.payParams || {}

  if (pay.devMode) {
    // 本地联调：后端 dev 回退，使用鉴权接口模拟当前用户订单支付。
    await request({
      url: '/miniapp/report/dev-pay',
      method: 'POST',
      auth: true,
      data: { out_trade_no: order.outTradeNo, trade_state: 'SUCCESS' },
    })
    return { ok: true, dev: true }
  }

  // 真实支付：拉起微信收银台。
  await new Promise((resolve, reject) => {
    uni.requestPayment({
      provider: 'wxpay',
      timeStamp: pay.timeStamp,
      nonceStr: pay.nonceStr,
      package: pay.package,
      signType: pay.signType || 'RSA',
      paySign: pay.paySign,
      success: () => resolve(),
      fail: (err) => reject(err),
    })
  })
  return { ok: true, dev: false }
}
