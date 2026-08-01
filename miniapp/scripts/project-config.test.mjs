import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const projectConfig = JSON.parse(readFileSync(resolve('project.config.json'), 'utf8'))
const manifest = JSON.parse(readFileSync(resolve('src/manifest.json'), 'utf8'))
const packageJson = JSON.parse(readFileSync(resolve('package.json'), 'utf8'))
const pagesConfig = JSON.parse(readFileSync(resolve('src/pages.json'), 'utf8'))

const pageTitles = Object.fromEntries(
  pagesConfig.pages.map(({ path, style }) => [path, style?.navigationBarTitleText]),
)
assert.equal(pageTitles['pages/index/index'], '首页', 'home navigation title should use the primary navigation label')
assert.equal(pageTitles['pages/learn/learn'], '学习中心', 'learn navigation title should describe the learning center')
assert.equal(pagesConfig.tabBar?.selectedColor, '#335B4A', 'selected tabs should use the brand green')
assert.equal(pagesConfig.tabBar?.backgroundColor, '#FFFDF8', 'native tab bar should use the warm surface color')
assert.equal(
  Object.hasOwn(pagesConfig.tabBar || {}, 'custom'),
  false,
  'primary navigation should use the native tab bar without a custom tab bar flag',
)
assert.deepEqual(
  pagesConfig.tabBar?.list,
  [
    {
      pagePath: 'pages/index/index',
      text: '首页',
      iconPath: 'static/tabbar/test.png',
      selectedIconPath: 'static/tabbar/test-active-green.png',
    },
    {
      pagePath: 'pages/learn/learn',
      text: '学习',
      iconPath: 'static/tabbar/learn.png',
      selectedIconPath: 'static/tabbar/learn-active-green.png',
    },
    {
      pagePath: 'pages/booking/booking',
      text: '预约',
      iconPath: 'static/tabbar/booking.png',
      selectedIconPath: 'static/tabbar/booking-active-green.png',
    },
    {
      pagePath: 'pages/profile/profile',
      text: '我的',
      iconPath: 'static/tabbar/profile.png',
      selectedIconPath: 'static/tabbar/profile-active-green.png',
    },
  ],
  'native tab bar should expose the exact four primary destinations and icon assets',
)

assert.notEqual(
  projectConfig?.setting?.urlCheck,
  false,
  'project.config.json must keep WeChat urlCheck enabled for release safety',
)

assert.notEqual(
  manifest?.['mp-weixin']?.setting?.urlCheck,
  false,
  'manifest mp-weixin urlCheck must keep WeChat domain validation enabled',
)

assert.equal(
  packageJson.scripts['prebuild:h5'],
  'node scripts/verify-production-api-base.mjs',
  'H5 production builds must verify VITE_API_BASE before generating a runnable-but-broken bundle',
)


const qaPath = resolve('QA.md')
assert.equal(existsSync(qaPath), true, 'QA.md should document miniapp real-device smoke checks')
const qa = readFileSync(qaPath, 'utf8')
for (const keyword of ['chooseAvatar', 'getPhoneNumber', 'requestPayment', 'canvas', 'share', 'mp-weixin']) {
  assert.match(qa, new RegExp(keyword), `QA.md should cover ${keyword} smoke check`)
}

const productionExamplePath = resolve('.env.production.example')
assert.equal(
  existsSync(productionExamplePath),
  true,
  '.env.production.example should document the required real HTTPS VITE_API_BASE for CI/release builds',
)
const productionExample = readFileSync(productionExamplePath, 'utf8')
assert.match(productionExample, /VITE_API_BASE=https:\/\/xn--9iq9az5uo8fz16d\.com\/api/)
assert.match(productionExample, /CI|release|上线|生产/)

const productionCheck = readFileSync(resolve('scripts/verify-production-api-base.mjs'), 'utf8')
assert.match(productionCheck, /\.local/, 'production API validation should reject .local placeholder/internal hosts')
assert.match(productionCheck, /yourdomain\.com/, 'production API validation should reject unchanged example hosts')
assert.doesNotMatch(qa, /\nnpm run build:h5\n/, 'QA automation should not suggest production H5 build without VITE_API_BASE')
assert.doesNotMatch(qa, /nine-xing\.local/, 'QA automation should not use .local placeholder API hosts')

console.log('project config tests passed')
