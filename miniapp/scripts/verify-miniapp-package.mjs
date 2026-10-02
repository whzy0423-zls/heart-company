import assert from 'node:assert/strict'
import { readFile, readdir, rm, stat } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import shareAssets from '../src/data/shareAssets.js'
import { resolveContentAsset } from '../src/utils/contentAsset.js'
import { normalizeClassroomContent, normalizeClassroomSeries } from '../src/utils/classroomDisplay.js'
import { buildShareCard } from '../src/utils/share.js'
import { readPackageLayout, verifyPackageSizes, WECHAT_PACKAGE_LIMIT, WECHAT_TOTAL_PACKAGE_LIMIT } from './wechat-package-layout.mjs'

const mode = process.argv[2]
assert.ok(['--sources', '--prune'].includes(mode), 'Usage: verify-miniapp-package.mjs --sources|--prune')
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const sourceRoot = path.join(root, 'src')
const outputRoot = path.join(root, 'dist/build/mp-weixin')
const publicRoot = path.resolve(root, '../website-react/public')
const manifest = JSON.parse(await readFile(path.join(sourceRoot, 'manifest.json'), 'utf8'))
const project = JSON.parse(await readFile(path.join(root, 'project.config.json'), 'utf8'))
const pageConfig = JSON.parse(await readFile(path.join(sourceRoot, 'pages.json'), 'utf8'))
const sourceLayout = readPackageLayout(pageConfig)
const options = manifest['mp-weixin'].packOptions
assert.deepEqual(project.packOptions, options, 'source project and generated WeChat package must use the same exclusions')

async function filesUnder(directory) {
  const files = []
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const filename = path.join(directory, entry.name)
    if (entry.isDirectory()) files.push(...await filesUnder(filename))
    else if (entry.isFile()) files.push(filename)
  }
  return files
}

const sourceFiles = await filesUnder(sourceRoot)
const runtimeSources = await Promise.all(sourceFiles.filter(filename => /\.(?:vue|js|json)$/.test(filename))
  .map(filename => readFile(filename, 'utf8')))
const excluded = []
for (const rule of options.ignore) {
  assert.ok(['folder', 'file'].includes(rule.type), 'package checker supports only explicit file/folder exclusions')
  assert.ok(rule.value.startsWith('static/') && !rule.value.includes('..'), 'exclusions must stay inside static assets')
  const filename = path.join(sourceRoot, rule.value)
  excluded.push(...(rule.type === 'folder' ? await filesUnder(filename) : [filename]))
}

let excludedBytes = 0
for (const filename of excluded) {
  const relative = path.relative(sourceRoot, filename).split(path.sep).join('/')
  const asset = `/${relative}`
  const source = await readFile(filename)
  excludedBytes += source.length
  if (asset.endsWith('.webp')) {
    assert.match(asset, /^\/static\/(?:enneagram\/[1-9]|editorial\/center-(?:gut|head|heart))\.webp$/)
    assert.ok(runtimeSources.every(text => !text.includes(asset)), `${asset} is referenced and must stay packaged`)
    // These are obsolete WebP siblings. The active PNG stays in the package.
    assert.ok((await stat(filename.replace(/\.webp$/, '.png'))).size > 0)
    continue
  }
  const published = shareAssets[asset]
  assert.ok(published, `${asset} needs a published original before it can leave the package`)
  assert.deepEqual(await readFile(path.join(publicRoot, published)), source, `${asset} must retain its original bytes online`)
  const resolved = resolveContentAsset(asset)
  assert.match(resolved, /^https:\/\/[^/]+\/assets\/miniapp-share\/.+\.jpg$/)
  assert.ok(resolved.endsWith(published), `${asset} should use its exact immutable original`)
  assert.equal(normalizeClassroomContent({ coverUrl: asset }).coverUrl, resolved)
  assert.equal(normalizeClassroomSeries({ coverUrl: asset }).coverUrl, resolved)
  assert.equal(buildShareCard({ kind: 'content', id: 21, imageUrl: asset }).appMessage.imageUrl, resolved)
}

const home = await readFile(path.join(sourceRoot, 'pages/index/index.vue'), 'utf8')
assert.match(home, /:src="resolveContentAsset\(item\.coverUrl\)"/, 'home fallback covers must resolve the same hosted assets as classroom pages')
const preserved = [
  'static/teacher/portrait.jpg', 'static/teacher/hero-portrait.jpg',
  ...Array.from({ length: 9 }, (_, index) => `static/enneagram/${index + 1}.png`),
  ...['gut', 'head', 'heart'].map(center => `static/editorial/center-${center}.png`),
]
for (const relative of preserved) {
  assert.ok(!excluded.includes(path.join(sourceRoot, relative)))
  assert.equal(resolveContentAsset(`/${relative}`), `/${relative}`, 'portraits and canvas assets must remain local')
}

if (mode === '--prune') {
  const builtProject = JSON.parse(await readFile(path.join(outputRoot, 'project.config.json'), 'utf8'))
  assert.deepEqual(builtProject.packOptions, options, 'production project must carry the verified exclusions')
  const appConfig = JSON.parse(await readFile(path.join(outputRoot, 'app.json'), 'utf8'))
  assert.deepEqual(readPackageLayout(appConfig), sourceLayout, 'compiled main/subpackage routes must match pages.json')
  for (const rule of options.ignore) await rm(path.join(outputRoot, rule.value), { recursive: true, force: true })
  for (const relative of preserved) assert.deepEqual(
    await readFile(path.join(outputRoot, relative)), await readFile(path.join(sourceRoot, relative)),
    `${relative} must stay unmodified for preview/canvas`,
  )
  const outputFiles = await filesUnder(outputRoot)
  const sizes = await Promise.all(outputFiles.map(async filename => ({
    path: path.relative(outputRoot, filename).split(path.sep).join('/'),
    size: (await stat(filename)).size,
  })))
  const { packages, totalBytes } = verifyPackageSizes(sizes, sourceLayout)
  for (const pkg of packages) console.log(`Verified WeChat ${pkg.root || 'main'} package: ${pkg.bytes} / ${WECHAT_PACKAGE_LIMIT} bytes; ${WECHAT_PACKAGE_LIMIT - pkg.bytes} bytes free`)
  console.log(`Verified aggregate package: ${totalBytes} / ${WECHAT_TOTAL_PACKAGE_LIMIT} bytes; ${excludedBytes} redundant asset bytes excluded`)
} else {
  console.log(`Verified ${excluded.length} package exclusions (${excludedBytes} bytes); all hosted originals and local preview/canvas assets preserved`)
}
