import assert from 'node:assert/strict'
import { readPackageLayout, verifyPackageSizes, WECHAT_PACKAGE_LIMIT, WECHAT_TOTAL_PACKAGE_LIMIT } from './wechat-package-layout.mjs'

const source = {
  pages: [{ path: 'pages/index/index' }],
  subPackages: [{ root: 'pages/relation', pages: [{ path: 'relation' }] }],
  tabBar: { list: [{ pagePath: 'pages/index/index' }] },
}
const layout = readPackageLayout(source)
assert.deepEqual(layout, readPackageLayout({
  pages: ['pages/index/index'],
  subPackages: [{ root: 'pages/relation', pages: ['relation'] }],
}))
const sizes = verifyPackageSizes([
  { path: 'common/vendor.js', size: WECHAT_PACKAGE_LIMIT - 100 },
  { path: 'pages/relation-other/data.js', size: 100 },
  { path: 'pages/relation/relation.js', size: 500_000 },
], layout)
assert.equal(sizes.packages[0].bytes, WECHAT_PACKAGE_LIMIT, 'a similarly named directory must remain in main')
assert.equal(sizes.packages[1].bytes, 500_000)
assert.ok(sizes.totalBytes > WECHAT_PACKAGE_LIMIT, 'a valid multi-package build can exceed 2 MiB in aggregate')
for (const filename of ['main.js', 'pages/relation/data.js']) {
  assert.throws(() => verifyPackageSizes([{ path: filename, size: WECHAT_PACKAGE_LIMIT + 1 }], layout), /exceeds 2 MiB/)
}
const manyPackages = readPackageLayout({ pages: ['index'], subPackages: Array.from({ length: 16 }, (_, i) => ({ root: `package${i}`, pages: ['index'] })) })
assert.throws(() => verifyPackageSizes(manyPackages.subPackages.map(pkg => ({ path: `${pkg.root}/data.js`, size: WECHAT_PACKAGE_LIMIT })), manyPackages), /aggregate package exceeds/)
assert.equal(WECHAT_TOTAL_PACKAGE_LIMIT, 30 * 1024 * 1024)
for (const root of ['../relation', '/relation', 'pages/../relation', 'pages//relation', 'pages/relation/', 'pages\\relation', './relation']) {
  assert.throws(() => readPackageLayout({ ...source, subPackages: [{ root, pages: ['relation'] }] }))
}
for (const secondRoot of ['pages/relation', 'pages/relation/child', 'pages']) {
  assert.throws(() => readPackageLayout({ ...source, subPackages: [...source.subPackages, { root: secondRoot, pages: ['extra'] }] }), /overlap|main pages/)
}
assert.throws(() => readPackageLayout({ ...source, pages: [{ path: 'pages/relation/relation' }] }), /main pages/)
assert.throws(() => readPackageLayout({ ...source, subPackages: [{ root: 'pages/relation', pages: ['relation', 'relation'] }] }), /unique/)
assert.throws(() => readPackageLayout({ ...source, tabBar: { list: [{ pagePath: 'pages/relation/relation' }] } }), /tabBar/)
assert.throws(() => verifyPackageSizes([{ path: '../main.js', size: 1 }], layout), /escape/)
assert.throws(() => verifyPackageSizes([{ path: 'main.js', size: 1 }, { path: 'main.js', size: 1 }], layout), /twice/)
console.log('WeChat package layout: same-route subpackage, separate budgets, aggregate cap and path boundaries passed')
