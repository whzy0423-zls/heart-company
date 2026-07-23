import assert from 'node:assert/strict'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, resolve } from 'node:path'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const scriptPath = resolve(dirname(fileURLToPath(import.meta.url)), 'verify-built-wechat-appid.mjs')
const productionAppId = 'wx7d12bddbec8e17f7'
const productionApiBase = 'https://xn--9iq9az5uo8fz16d.com/api'

function createBuild({ appid = productionAppId, js = `const API_BASE = '${productionApiBase}'` } = {}) {
  const root = mkdtempSync(resolve(tmpdir(), 'nine-xing-built-miniapp-'))
  const buildRoot = resolve(root, 'dist/build/mp-weixin')
  mkdirSync(resolve(buildRoot, 'common/vendor'), { recursive: true })
  writeFileSync(resolve(buildRoot, 'project.config.json'), JSON.stringify({ appid }))
  writeFileSync(resolve(buildRoot, 'common/vendor/index.js'), js)
  return root
}

function verify(root) {
  return spawnSync(process.execPath, [scriptPath], {
    cwd: root,
    encoding: 'utf8',
  })
}

function withBuild(options, callback) {
  const root = createBuild(options)
  try {
    callback(verify(root))
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
}

withBuild({}, (result) => {
  assert.equal(result.status, 0, result.stderr)
  assert.match(result.stdout, /verified production WeChat AppID and API base/i)
})

withBuild({ appid: 'wx-old-appid' }, (result) => {
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /AppID/)
})

withBuild({ js: "const API_BASE = 'https://api.example.com/api'" }, (result) => {
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /production API base/)
})

for (const forbidden of [
  'https://api.example.com/api',
  'https://api.yourdomain.com/api',
  'https://nine-xing.local/api',
]) {
  withBuild({ js: `const PROD = '${productionApiBase}'; const BAD = '${forbidden}'` }, (result) => {
    assert.notEqual(result.status, 0)
    assert.match(result.stderr, /forbidden API host/)
  })
}

console.log('built WeChat config verifier tests passed')
