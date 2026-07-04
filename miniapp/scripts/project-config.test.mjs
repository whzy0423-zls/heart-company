import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const projectConfig = JSON.parse(readFileSync(resolve('project.config.json'), 'utf8'))
const manifest = JSON.parse(readFileSync(resolve('src/manifest.json'), 'utf8'))

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


const qaPath = resolve('QA.md')
assert.equal(existsSync(qaPath), true, 'QA.md should document miniapp real-device smoke checks')
const qa = readFileSync(qaPath, 'utf8')
for (const keyword of ['chooseAvatar', 'getPhoneNumber', 'requestPayment', 'canvas', 'share', 'mp-weixin']) {
  assert.match(qa, new RegExp(keyword), `QA.md should cover ${keyword} smoke check`)
}

console.log('project config tests passed')
