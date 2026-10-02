import assert from 'node:assert/strict'
import path from 'node:path'

export const WECHAT_PACKAGE_LIMIT = 2 * 1024 * 1024
// https://developers.weixin.qq.com/miniprogram/dev/framework/subpackages.html
// Directly developed apps: 30M total; service-provider-developed apps: 20M.
export const WECHAT_TOTAL_PACKAGE_LIMIT = 30 * 1024 * 1024

function relativePath(value, label) {
  assert.ok(typeof value === 'string' && value.length > 0, `${label} must be a nonempty relative path`)
  assert.ok(value === value.trim() && !value.includes('\\') && !/[?#]/.test(value), `${label} must be a normalized path`)
  assert.ok(!path.posix.isAbsolute(value) && !value.split('/').some(segment => ['', '.', '..'].includes(segment)), `${label} must not escape or alias its package`)
  return value
}

export function readPackageLayout(config) {
  const pagePaths = (pages, label) => {
    assert.ok(Array.isArray(pages) && pages.length > 0, `${label} must contain pages`)
    return pages.map(page => relativePath(typeof page === 'string' ? page : page?.path, label))
  }
  const mainPages = pagePaths(config.pages, 'main pages')
  assert.ok(!(config.subPackages && config.subpackages), 'use only one subPackages spelling')
  const configuredPackages = config.subPackages || config.subpackages || []
  assert.ok(Array.isArray(configuredPackages), 'subPackages must be an array')
  const subPackages = configuredPackages.map(pkg => ({
    root: relativePath(pkg.root, 'subpackage root'),
    pages: pagePaths(pkg.pages, `subpackage ${pkg.root}`),
    independent: pkg.independent === true,
  }))
  const seenRoots = []
  for (const pkg of subPackages) {
    assert.ok(!seenRoots.some(root => root === pkg.root || root.startsWith(`${pkg.root}/`) || pkg.root.startsWith(`${root}/`)), `subpackage roots must not overlap: ${pkg.root}`)
    seenRoots.push(pkg.root)
    assert.ok(mainPages.every(page => page !== pkg.root && !page.startsWith(`${pkg.root}/`)), `main pages must stay outside subpackage ${pkg.root}`)
  }
  const allPages = [...mainPages, ...subPackages.flatMap(pkg => pkg.pages.map(page => `${pkg.root}/${page}`))]
  assert.equal(new Set(allPages).size, allPages.length, 'registered page paths must be unique')
  for (const item of config.tabBar?.list || []) {
    assert.ok(mainPages.includes(item.pagePath), `tabBar page must stay in the main package: ${item.pagePath}`)
  }
  return { mainPages, subPackages }
}

export function verifyPackageSizes(files, layout, totalLimit = WECHAT_TOTAL_PACKAGE_LIMIT) {
  const packages = [{ root: '', bytes: 0 }, ...layout.subPackages.map(pkg => ({ root: pkg.root, bytes: 0 }))]
  const seenFiles = new Set()
  for (const file of files) {
    const filename = relativePath(file.path, 'output file')
    assert.ok(!seenFiles.has(filename), `output file counted twice: ${filename}`)
    seenFiles.add(filename)
    assert.ok(Number.isSafeInteger(file.size) && file.size >= 0, `invalid output size: ${filename}`)
    const target = packages.find(pkg => pkg.root && filename.startsWith(`${pkg.root}/`)) || packages[0]
    target.bytes += file.size
  }
  for (const pkg of packages) {
    assert.ok(pkg.bytes <= WECHAT_PACKAGE_LIMIT, `WeChat ${pkg.root || 'main'} package exceeds 2 MiB: ${pkg.bytes} bytes`)
  }
  const totalBytes = packages.reduce((sum, pkg) => sum + pkg.bytes, 0)
  assert.ok(totalBytes <= totalLimit, `WeChat aggregate package exceeds ${totalLimit} bytes: ${totalBytes} bytes`)
  return { packages, totalBytes }
}
