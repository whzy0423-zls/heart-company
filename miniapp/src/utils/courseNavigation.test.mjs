import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'

const source = readFileSync(new URL('./courseNavigation.js', import.meta.url), 'utf8').replace(/^export /gm, '')
const page = (route, bookingId = '') => ({ route, options: { bookingId } })
function harness(pages = []) {
  const state = { pages, calls: [], toasts: [], errors: {}, pending: [] }
  const navigate = (method, options) => {
    state.calls.push({ method, ...options })
    const complete = () => {
      const failure = state.errors[method] || (method === 'navigateTo' && state.pages.length >= 10 ? 'webview count limit exceed' : '')
      if (failure) { options.fail?.({ errMsg: `${method}:fail ${failure}` }); return }
      if (method === 'navigateBack') state.pages.splice(-options.delta)
      else {
        const url = new URL(options.url, 'https://example.test')
        if (method === 'redirectTo') state.pages.pop()
        state.pages.push(page(url.pathname.slice(1), url.searchParams.get('bookingId') || ''))
      }
      options.success?.({ errMsg: `${method}:ok` })
    }
    if (state.defer) state.pending.push(complete)
    else complete()
  }
  const context = vm.createContext({
    getCurrentPages: () => state.pages,
    uni: Object.fromEntries(['navigateTo', 'redirectTo', 'navigateBack'].map(method => [method, options => navigate(method, options)]).concat([['showToast', options => state.toasts.push(options)]])),
  })
  vm.runInContext(`${source}\nglobalThis.navigation = { navigateToMyCourse, navigateToOrders, navigateToPaymentResult }`, context)
  return { state, ...context.navigation }
}
{
  const { state, navigateToMyCourse } = harness(Array.from({ length: 10 }, (_, i) => page(`pages/page-${i}`)))
  navigateToMyCourse('91')
  assert.equal(state.pages.at(-1).route, 'pages/my-course/my-course', 'a full stack must still open the purchased course')
  assert.equal(state.pages.length, 10)
  assert.equal(state.calls[0].method, 'redirectTo')
}
{
  const { state, navigateToMyCourse } = harness([page('pages/booking/booking'), page('pages/my-course/my-course', '91'), page('pages/course-detail/course-detail')])
  let completed = 0
  navigateToMyCourse('91', { complete: () => completed++ })
  assert.equal(state.calls[0].method, 'navigateBack')
  assert.equal(state.pages.length, 2)
  assert.equal(completed, 1)
  navigateToMyCourse('91', { complete: () => completed++ })
  assert.equal(state.calls.length, 1, 'opening the current course must not add a duplicate page')
  assert.equal(completed, 2, 'a no-op success must release caller loading state')
}
{
  const { state, navigateToMyCourse } = harness([page('pages/my-course/my-course', '90')])
  navigateToMyCourse('91')
  assert.equal(state.pages.length, 2, 'another purchased course must not reuse the wrong booking')
  assert.equal(state.pages.at(-1).options.bookingId, '91')
}
{
  const { state, navigateToMyCourse } = harness([page('pages/course-detail/course-detail')])
  state.errors.navigateTo = 'timeout'
  let completed = 0
  let succeeded = 0
  navigateToMyCourse('91', { success: () => succeeded++, complete: () => completed++ })
  assert.deepEqual(state.calls.map(call => call.method), ['navigateTo', 'redirectTo'])
  assert.equal(state.pages.at(-1).options.bookingId, '91')
  assert.equal(succeeded, 1)
  assert.equal(completed, 1)
}
{
  const { state, navigateToMyCourse } = harness([page('pages/course-detail/course-detail')])
  state.errors = { navigateTo: 'timeout', redirectTo: 'timeout' }
  let failed = 0
  let completed = 0
  navigateToMyCourse('91', { fail: () => failed++, complete: () => completed++ })
  assert.equal(failed, 1, 'final failure must be observable by the page')
  assert.equal(completed, 1, 'final failure must release caller loading state')
  assert.equal(state.calls.length, 2, 'retry at most once')
}
{
  const { state, navigateToMyCourse } = harness([page('pages/course-detail/course-detail')])
  state.defer = true
  state.errors.navigateTo = 'timeout'
  let current = true
  navigateToMyCourse('91', { isCurrent: () => current })
  current = false
  state.pending.shift()()
  assert.equal(state.calls.length, 1, 'late failure after leaving the session must not redirect')
  assert.equal(state.toasts.length, 0)
}
{
  const { state, navigateToMyCourse } = harness()
  assert.equal(navigateToMyCourse('forged'), false)
  assert.equal(state.calls.length, 0)
}
console.log('Course navigation passed: full stack, existing booking, timeout recovery, bounded failure and session cancellation')
