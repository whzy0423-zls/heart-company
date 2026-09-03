import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const appSource = readFileSync(resolve(__dirname, '../App.jsx'), 'utf8')
const configSource = readFileSync(resolve(__dirname, '../../../shared/site-config.json'), 'utf8')
const quotesSource = readFileSync(resolve(__dirname, 'Quotes.jsx'), 'utf8')
const typesSource = readFileSync(resolve(__dirname, 'Types.jsx'), 'utf8')
const signupSource = readFileSync(resolve(__dirname, 'Signup.jsx'), 'utf8')

for (const [name, page] of [['Quotes', 'quotes'], ['Types', 'types'], ['Signup', 'signup']]) {
  assert.match(appSource, new RegExp(`const\\s+${name}\\s*=\\s*lazy\\(\\(\\)\\s*=>\\s*import\\('\\./pages/${name}'\\)\\)`), `官网需要懒加载 ${page} 页面`)
  assert.match(appSource, new RegExp(`path="${page}"`), `官网需要 /${page} 路由`)
}

assert.match(configSource, /"label":\s*"老韩语录"[\s\S]*?"to":\s*"\/quotes"[\s\S]*?"type":\s*"route"/, '老韩语录导航需要跳转到 /quotes')
assert.match(configSource, /"label":\s*"九种芯片(?:模式)?"[\s\S]*?"to":\s*"\/types"[\s\S]*?"type":\s*"route"/, '九种芯片导航需要跳转到 /types')
assert.match(configSource, /"label":\s*"注册\/互动"[\s\S]*?"to":\s*"\/signup"[\s\S]*?"type":\s*"route"/, '注册/互动导航需要跳转到 /signup')

assert.doesNotMatch(configSource, /"label":\s*"老韩语录"[\s\S]*?"to":\s*"\/#quotes"/, '老韩语录导航不应再跳回首页锚点')
assert.doesNotMatch(configSource, /"label":\s*"九种芯片(?:模式)?"[\s\S]*?"to":\s*"\/#types"/, '九种芯片导航不应再跳回首页锚点')
assert.doesNotMatch(configSource, /"label":\s*"注册\/互动"[\s\S]*?"to":\s*"\/#signup"/, '注册/互动导航不应再跳回首页锚点')

assert.match(quotesSource, /QuotesSection/, '老韩语录页面需要复用语录区块')
assert.match(typesSource, /TypesSection/, '九种芯片页面需要复用芯片区块')
assert.match(signupSource, /SignupSection/, '注册/互动页面需要复用报名区块')

console.log('nav anchor route tests passed')
