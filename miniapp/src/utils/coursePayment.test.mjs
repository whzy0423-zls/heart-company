import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('./coursePayment.js', import.meta.url), 'utf8')
const { createCoursePaymentController, coursePaymentResultUrl } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)
const order = { bookingId: '52', outTradeNo: 'course-52', status: 'pending', payParams: { package: 'prepay_id=52' } }
const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done }); return { promise, resolve } }
const flush = async () => { for (let i = 0; i < 12; i++) await Promise.resolve() }
function harness(extra = {}) {
  const calls = { create: 0, pay: 0, status: 0, snapshots: [] }
  const controller = createCoursePaymentController({
    create: async () => { calls.create++; return order }, pay: async () => { calls.pay++ },
    status: async () => { calls.status++; return { status: 'paid' } },
    onChange: state => calls.snapshots.push(state), wait: async () => {}, ...extra,
  })
  return { calls, controller }
}
{
  const { calls, controller } = harness({ create: async () => ({ ...order, status: 'paid', payParams: undefined }) })
  assert.equal((await controller.purchase()).state, 'success')
  assert.equal(calls.pay, 0, 'backend-confirmed paid order must never open the cashier')
}
{
  const { calls, controller } = harness({ create: async () => ({ ...order, payParams: undefined, syncStatus: 'retrying' }), status: async () => ({ status: 'pending', syncStatus: 'retrying' }) })
  const result = await controller.purchase()
  assert.equal(calls.pay, 0, 'reconciliation uncertainty must never open the cashier')
  assert.equal(result.state, 'pending')
  assert.doesNotMatch(result.message, /重新支付|失败|重试支付/)
}
{
  const { controller, calls } = harness({ pay: () => new Promise(() => {}), paymentTimeoutMs: 5 })
  const result = await controller.purchase()
  assert.equal(result.state, 'success', 'missing native callback still queries backend after bounded wait')
  assert.equal(calls.status, 1)
}
{
  const { controller, calls } = harness({ pay: () => new Promise(() => {}), paymentTimeoutMs: 60000 })
  const buying = controller.purchase()
  assert.equal(controller.purchase(), buying, 'double taps share one operation')
  await flush()
  controller.resume()
  controller.resume()
  assert.equal((await buying).state, 'success', 'wallet return recovers without native callback')
  assert.equal(calls.create, 1)
  assert.equal(calls.status, 1, 'repeated onShow does not create duplicate confirmation loops')
}
for (const errMsg of ['requestPayment:fail cancel', 'requestPayment:fail network']) {
  const { controller, calls } = harness({ pay: async () => { throw { errMsg } } })
  assert.equal((await controller.purchase()).state, 'success', 'native outcome never overrides server payment truth')
  assert.equal(calls.status, 1)
}
{
  const { controller, calls } = harness({ status: async () => { calls.status++; return { status: 'pending' } } })
  assert.equal((await controller.purchase()).state, 'pending', 'confirmation exhaustion is not payment failure')
  assert.equal(calls.status, 6)
}
{
  const { controller } = harness({ status: async () => { throw new Error('Network') } })
  assert.equal((await controller.purchase()).state, 'pending', 'network failure keeps outcome unknown')
}
{
  const { controller } = harness({ status: async () => ({ status: 'closed', syncStatus: 'retrying' }) })
  assert.equal((await controller.purchase()).state, 'pending', 'unconfirmed provider state takes priority over a stale local close')
}
{
  let current = true
  const { controller } = harness({ isCurrent: () => current, status: async () => { current = false; return { status: 'paid' } } })
  assert.equal((await controller.purchase()).state, 'stopped', 'account switch during confirmation cannot publish success')
}
{
  let current = true
  const gate = deferred()
  const { controller, calls } = harness({ create: () => gate.promise, isCurrent: () => current })
  const buying = controller.purchase()
  current = false
  gate.resolve(order)
  assert.equal((await buying).state, 'stopped')
  assert.equal(calls.pay, 0, 'account change during creation does not open old cashier')
}
{
  const { controller, calls } = harness({ pay: () => new Promise(() => {}), paymentTimeoutMs: 60000 })
  const buying = controller.purchase()
  await flush()
  controller.stop()
  assert.equal((await buying).state, 'stopped', 'unload releases native callback wait')
  assert.equal(calls.status, 0)
}
assert.equal(coursePaymentResultUrl({ bookingId: '52', status: 'paid', amount: 1 }), '/pages/payment-result/payment-result?bookingId=52')
assert.equal(coursePaymentResultUrl({ bookingId: 'bad' }), '')
console.log('Course payment recovery and backend-truth tests passed')
