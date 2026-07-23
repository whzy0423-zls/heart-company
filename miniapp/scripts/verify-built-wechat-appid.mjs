import { existsSync, readFileSync, readdirSync } from 'node:fs'
import { createRequire } from 'node:module'
import { extname, resolve } from 'node:path'

const buildRoot = resolve('dist/build/mp-weixin')
const projectConfigPath = resolve(buildRoot, 'project.config.json')
const builtConfigPath = resolve(buildRoot, 'config.js')
const productionAppId = 'wx7d12bddbec8e17f7'
const productionApiBase = 'https://xn--9iq9az5uo8fz16d.com/api'
const forbiddenApiHosts = [
  /api\.example\.com/i,
  /(?:api\.)?yourdomain\.com/i,
  /\.local\/api(?:\b|\/)/i,
]

function fail(message) {
  console.error(message)
  process.exit(1)
}

function findJavaScriptFiles(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = resolve(directory, entry.name)
    if (entry.isDirectory()) return findJavaScriptFiles(path)
    return entry.isFile() && extname(entry.name) === '.js' ? [path] : []
  })
}

if (!existsSync(projectConfigPath)) {
  fail(`Built WeChat project config not found: ${projectConfigPath}`)
}

const projectConfig = JSON.parse(readFileSync(projectConfigPath, 'utf8'))
if (projectConfig.appid !== productionAppId) {
  fail(`Built WeChat AppID must be ${productionAppId}.`)
}

const javaScriptFiles = findJavaScriptFiles(buildRoot)
const builtJavaScript = javaScriptFiles.map((path) => readFileSync(path, 'utf8')).join('\n')

if (!existsSync(builtConfigPath)) {
  fail(`Built miniapp API config not found: ${builtConfigPath}`)
}

let effectiveApiBase
try {
  const require = createRequire(import.meta.url)
  effectiveApiBase = require(builtConfigPath).API_BASE
} catch (error) {
  fail(`Unable to read the effective production API base: ${error.message}`)
}

if (effectiveApiBase !== productionApiBase) {
  fail(`Built miniapp effective production API base must be ${productionApiBase}.`)
}

if (forbiddenApiHosts.some((pattern) => pattern.test(builtJavaScript))) {
  fail('Built JavaScript contains a forbidden API host.')
}

console.log('Verified production WeChat AppID and API base.')
