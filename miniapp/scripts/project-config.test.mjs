import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
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

console.log('project config tests passed')
