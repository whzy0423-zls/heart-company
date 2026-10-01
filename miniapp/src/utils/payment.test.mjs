import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { dirname, join, resolve } from 'node:path'
import { tmpdir } from 'node:os'
import vm from 'node:vm'

const __dirname = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(resolve(__dirname, './payment.js'), 'utf8')

// uni-app injects a module-scoped uni runtime in the WeChat build. It need not
// exist on globalThis. Exercise that shape rather than only a Node global mock.
{
  const calls = []
  const runtime = {
    requestPayment(options) {
      assert.equal(this, runtime, 'keep the API receiver when invoking payment')
      calls.push(options)
      options.success({ errMsg: 'requestPayment:ok' })
    },
  }
  const context = vm.createContext({ injectedRuntime: runtime })
  const script = source.replace(/^import[^\n]+\n/gm, '').replace(/\bexport /g, '')
  vm.runInContext(`const uni = injectedRuntime;\n${script}\nglobalThis.paymentEntry = requestWechatPayment`, context)
  assert.equal(context.uni, undefined, 'runtime is lexical, not a global property')
  await context.paymentEntry({ timeStamp: '1700000000', nonceStr: 'nonce', package: 'prepay_id=1', paySign: 'signature' })
  assert.equal(calls.length, 1, 'the injected WeChat runtime must open the cashier')
  assert.equal(calls[0].provider, 'wxpay')
}

assert.match(
  source,
  /url:\s*['"]\/miniapp\/report\/dev-pay['"]/,
  'dev payment simulation must use the authenticated miniapp endpoint',
)
assert.doesNotMatch(
  source,
  /url:\s*['"]\/pay\/notify['"]/,
  'miniapp must not call the public wxpay callback endpoint',
)

assert.match(source, /export (?:async )?function requestWechatPayment\(/)
assert.match(source, /export (?:async )?function payWechatOrder\(/)
assert.match(source, /export function createWechatPaymentController\(/)

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-payment-'))
try {
  let paymentSource = await readFile(new URL('./payment.js', import.meta.url), 'utf8')
  const controllerSource = await readFile(new URL('./classroomProgress.js', import.meta.url), 'utf8')
  await writeFile(join(dir, 'classroomProgress.mjs'), controllerSource)
  paymentSource = paymentSource
    .replace(
      "import { request } from '../api/request'",
      'const request = globalThis.__paymentHarness.request',
    )
    .replace(
      "import { createReportOrderApi, reportStatusApi } from '../api'",
      'const createReportOrderApi = globalThis.__paymentHarness.createReportOrderApi\nconst reportStatusApi = globalThis.__paymentHarness.reportStatusApi',
    )
    .replace(
      "import { createWechatPaymentController as createOrderController } from './classroomProgress'",
      "import { createWechatPaymentController as createOrderController } from './classroomProgress.mjs'",
    )
  await writeFile(join(dir, 'payment.mjs'), paymentSource)

  const paymentCalls = []
  globalThis.__paymentHarness = {
    request: async (...args) => {
      paymentCalls.push(args)
      return { ok: true }
    },
    createReportOrderApi: async () => ({
      outTradeNo: 'report-1',
      payParams: { devMode: true },
    }),
    reportStatusApi: async () => ({ unlocked: true }),
  }
  const requestPaymentCalls = []
  globalThis.uni = {
    requestPayment(options) {
      requestPaymentCalls.push(options)
      options.success({ errMsg: 'requestPayment:ok' })
    },
  }

  const payment = await import(`file://${join(dir, 'payment.mjs')}`)
  await assert.rejects(
    payment.requestWechatPayment({ package: 'prepay_id=missing' }),
    /支付参数不完整/,
    'missing signed fields must fail before opening the cashier',
  )

  await payment.requestWechatPayment({
    timeStamp: '1700000000',
    nonceStr: 'nonce',
    package: 'prepay_id=1',
    signType: 'RSA',
    paySign: 'signature',
  })
  assert.equal(requestPaymentCalls.length, 1)
  assert.deepEqual({
    ...requestPaymentCalls[0],
    success: undefined,
    fail: undefined,
  }, {
    provider: 'wxpay',
    timeStamp: '1700000000',
    nonceStr: 'nonce',
    package: 'prepay_id=1',
    signType: 'RSA',
    paySign: 'signature',
    success: undefined,
    fail: undefined,
  })

  await payment.payWechatOrder(
    { outTradeNo: 'dev-1', payParams: { devMode: true } },
    { devPay: async (order) => ({ outTradeNo: order.outTradeNo, simulated: true }) },
  )
  assert.equal(requestPaymentCalls.length, 1, 'dev payment must not open the real cashier')

  const reportResult = await payment.payForReport('record-1')
  assert.equal(reportResult.ok, true)
  assert.equal(reportResult.dev, true)
  assert.equal(paymentCalls.length, 1)
  assert.deepEqual(paymentCalls[0][0], {
    url: '/miniapp/report/dev-pay',
    method: 'POST',
    auth: true,
    data: { out_trade_no: 'report-1', trade_state: 'SUCCESS' },
  })
} finally {
  delete globalThis.__paymentHarness
  delete globalThis.uni
  await rm(dir, { force: true, recursive: true })
}

console.log('payment dev simulation tests passed')
