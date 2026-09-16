import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const privacyPath = resolve('public/legal/privacy-policy.html')
const rightsPath = resolve('public/legal/privacy-rights.html')
const footerSource = readFileSync(resolve('src/components/Footer.jsx'), 'utf8')

assert.ok(existsSync(privacyPath), '官网需要提供可公网直访的隐私政策静态页')
assert.ok(existsSync(rightsPath), '官网需要提供可公网直访的隐私权利静态页')

const privacy = readFileSync(privacyPath, 'utf8')
const rights = readFileSync(rightsPath, 'utf8')

for (const text of ['九型芯之力 App 隐私政策', '烟台智伽文化科技有限公司', '个人信息收集清单', '第三方 SDK', '账号注销']) {
  assert.match(privacy, new RegExp(text), `隐私政策需要包含 ${text}`)
}

for (const text of ['隐私权利说明', '访问、更正、删除', '撤回同意', '注销账号', '联系客服']) {
  assert.match(rights, new RegExp(text), `隐私权利页需要包含 ${text}`)
}

assert.match(footerSource, /href=["']\/legal\/privacy-policy\.html["']/, '页脚需要链接隐私政策静态页')
assert.match(footerSource, /href=["']\/legal\/privacy-rights\.html["']/, '页脚需要链接隐私权利静态页')

console.log('legal static pages tests passed')
