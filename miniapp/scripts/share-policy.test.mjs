import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { readPackageLayout } from './wechat-package-layout.mjs'

const publicPages = ['index','teacher','learn','course-detail','classroom','classroom-detail','result','enneagram','enneagram-detail','relation']
const privatePages = ['booking','booking-records','booking-detail','orders','my-course','payment-result','profile','profile-edit','test']
const pageConfig = JSON.parse(await readFile(new URL('../src/pages.json',import.meta.url)))
const layout = readPackageLayout(pageConfig)
const routes = [...layout.mainPages, ...layout.subPackages.flatMap(pkg => pkg.pages.map(page => `${pkg.root}/${page}`))]
assert.deepEqual(routes.sort(), [...publicPages,...privatePages].map(page => `pages/${page}/${page}`).sort(), 'Every registered main/subpackage page needs an explicit sharing policy')
for (const page of privatePages) {
  const source = await readFile(new URL(`../src/pages/${page}/${page}.vue`,import.meta.url),'utf8')
  assert.match(source, /hideShareMenu/, `${page} must hide native sharing on entry`)
  assert.doesNotMatch(source, /onShareAppMessage\s*\(|onShareTimeline\s*\(|open-type="share"/, `${page} contains private customer data`)
}
for (const page of publicPages) {
  const source = await readFile(new URL(`../src/pages/${page}/${page}.vue`,import.meta.url),'utf8')
  for (const hook of ['onShareAppMessage','onShareTimeline']) assert.match(source,new RegExp(`${hook}\\s*\\(`), `${page} must register ${hook} directly for uni-app compilation`)
  if (process.argv.includes('--compiled')) {
    const compiled = await readFile(new URL(`../dist/build/mp-weixin/pages/${page}/${page}.js`,import.meta.url),'utf8')
    assert.match(compiled, /__runtimeHooks\s*[:=]\s*6\b/, `${page} must emit both WeChat share hooks`)
  }
}
console.log('All 19 page sharing policies verified' + (process.argv.includes('--compiled') ? ' in compiled WeChat bundle' : ''))
