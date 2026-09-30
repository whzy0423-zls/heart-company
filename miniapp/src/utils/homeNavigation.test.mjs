import assert from 'node:assert/strict'
import { resolveHomeNavigation } from './homeNavigation.js'

const wechat = { wechat: true }
const unavailable = () => { throw new Error('API unavailable') }

{
  let legacyCalls = 0
  const layout = resolveHomeNavigation({
    getWindowInfo: () => ({ statusBarHeight: 44, windowWidth: 375 }),
    getSystemInfoSync: () => { legacyCalls++; return { statusBarHeight: 20 } },
    getMenuButtonBoundingClientRect: () => ({ left: 278, right: 365, top: 48, bottom: 80, width: 87, height: 32 }),
  }, wechat)
  assert.deepEqual(layout, { statusBarHeight: 44, capsuleInset: 109 }, 'iOS should protect the status bar and the complete capsule plus its gap')
  assert.equal(legacyCalls, 0, 'valid modern geometry should avoid the deprecated system API')
}

assert.deepEqual(resolveHomeNavigation({
  getWindowInfo: () => ({ statusBarHeight: 24, windowWidth: 360 }),
  getMenuButtonBoundingClientRect: () => ({ left: 263, right: 350, top: 28, bottom: 60, width: 87, height: 32 }),
}, wechat), { statusBarHeight: 24, capsuleInset: 109 }, 'Android should retain its own status bar height')

assert.deepEqual(resolveHomeNavigation({
  getWindowInfo: unavailable,
  getSystemInfoSync: () => ({ statusBarHeight: 20, windowWidth: 320 }),
  getMenuButtonBoundingClientRect: () => ({ left: 223, right: 310, top: 24, bottom: 56 }),
}, wechat), { statusBarHeight: 20, capsuleInset: 109 }, 'a failing modern API should still allow legacy geometry')

assert.deepEqual(resolveHomeNavigation({
  getWindowInfo: () => ({ statusBarHeight: 0, windowWidth: 390 }),
  getSystemInfoSync: () => ({ statusBarHeight: 47 }),
  getMenuButtonBoundingClientRect: () => ({ left: 293, right: 380, top: 51, bottom: 83 }),
}, wechat), { statusBarHeight: 47, capsuleInset: 109 }, 'zero modern height should fall back to a positive legacy value')

{
  let legacyCalls = 0
  assert.deepEqual(resolveHomeNavigation({
    getWindowInfo: () => ({ statusBarHeight: 0, safeArea: { top: 59 }, windowWidth: 393 }),
    getSystemInfoSync: () => { legacyCalls++; return { statusBarHeight: 20 } },
  }, wechat), { statusBarHeight: 59, capsuleInset: 104 }, 'modern safe-area geometry should avoid the deprecated API')
  assert.equal(legacyCalls, 0)
}

assert.deepEqual(resolveHomeNavigation({
  getWindowInfo: () => ({ statusBarHeight: 0, safeArea: { top: 59 }, windowWidth: 393 }),
  getSystemInfoSync: unavailable,
  getMenuButtonBoundingClientRect: () => ({ left: 296, right: 383, top: 63, bottom: 95 }),
}, wechat), { statusBarHeight: 59, capsuleInset: 109 }, 'a failing legacy API must not discard a valid modern safe area')

assert.deepEqual(resolveHomeNavigation({
  getWindowInfo: () => ({ statusBarHeight: 0 }),
  getSystemInfoSync: () => ({ statusBarHeight: 0, safeAreaInsets: { top: 44 }, windowWidth: 375 }),
  getMenuButtonBoundingClientRect: unavailable,
}, wechat), { statusBarHeight: 44, capsuleInset: 104 }, 'native safe-area insets should remain usable if capsule measurement throws')

assert.deepEqual(resolveHomeNavigation({
  getWindowInfo: () => ({ statusBarHeight: 0, windowWidth: 375 }),
  getSystemInfoSync: unavailable,
  getMenuButtonBoundingClientRect: () => ({ left: 278, right: 365, top: 48, bottom: 80 }),
}, wechat), { statusBarHeight: 44, capsuleInset: 109 }, 'capsule top should provide a last native status-bar estimate')

assert.deepEqual(resolveHomeNavigation({
  getWindowInfo: () => ({ statusBarHeight: 44, windowWidth: -1 }),
  getSystemInfoSync: () => ({ windowWidth: 375 }),
  getMenuButtonBoundingClientRect: () => ({ left: 278, top: 48 }),
}, wechat), { statusBarHeight: 44, capsuleInset: 104 }, 'a missing modern width should use the conservative capsule inset without calling legacy')

for (const invalid of [undefined, null, NaN, Infinity, -1, 0, '', 'bad']) {
  assert.deepEqual(resolveHomeNavigation({
    getWindowInfo: () => ({ statusBarHeight: invalid, windowWidth: invalid }),
    getSystemInfoSync: () => ({ statusBarHeight: invalid }),
    getMenuButtonBoundingClientRect: () => ({ left: invalid, top: invalid }),
  }, wechat), { statusBarHeight: 44, capsuleInset: 104 }, 'invalid dimensions should have finite conservative defaults')
}

for (const left of [-2, 0, 375, 400, Infinity, NaN]) {
  assert.deepEqual(resolveHomeNavigation({
    getWindowInfo: () => ({ statusBarHeight: 44, windowWidth: 375 }),
    getMenuButtonBoundingClientRect: () => ({ left, top: 48 }),
  }, wechat), { statusBarHeight: 44, capsuleInset: 104 }, 'capsule positions outside the viewport should not hide the brand')
}

assert.deepEqual(resolveHomeNavigation(undefined, wechat), { statusBarHeight: 44, capsuleInset: 104 }, 'an absent native bridge should reserve safe space')
assert.deepEqual(resolveHomeNavigation({
  getWindowInfo: () => ({ platform: 'ios', statusBarHeight: 44, windowWidth: 375 }),
  getSystemInfoSync: unavailable,
  getMenuButtonBoundingClientRect: unavailable,
}), { statusBarHeight: 0, capsuleInset: 0 }, 'H5 on iOS should not acquire native WeChat insets')

console.log('home navigation geometry tests passed')
