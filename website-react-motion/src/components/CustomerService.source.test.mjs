import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import assert from 'node:assert/strict'

const __dirname = dirname(fileURLToPath(import.meta.url))
const component = readFileSync(resolve(__dirname, './CustomerService.jsx'), 'utf8')
const layout = readFileSync(resolve(__dirname, './Layout.jsx'), 'utf8')
const css = readFileSync(resolve(__dirname, '../index.css'), 'utf8')
const config = JSON.parse(readFileSync(resolve(__dirname, '../../../shared/site-config.json'), 'utf8'))

test('客服二维码入口接入全局布局并使用指定弹窗标题', () => {
  assert.match(layout, /import CustomerService from '\.\/CustomerService'/)
  assert.match(layout, /<CustomerService \/>/)
  assert.match(component, /芯之力 小助手/)
  assert.match(component, /siteConfig\?\.site\?\.customerServiceQr/)
  assert.match(component, /\/public\/customer-service-qr/)
  assert.match(css, /\.customer-service-fab/)
  assert.match(css, /\.customer-service-modal/)
  assert.equal(typeof config.site.customerServiceQr, 'string')
  assert.ok(config.site.customerServiceQr.length > 0)
})
