import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const homeSource = readFileSync(resolve(__dirname, './Home.jsx'), 'utf8')
const siteConfig = JSON.parse(readFileSync(resolve(__dirname, '../../../shared/site-config.json'), 'utf8'))

test('places the App download section immediately after Hero and before the teacher teaser', () => {
  assert.match(homeSource, /import AppDownloadSection from '\.\.\/components\/AppDownloadSection'/)
  assert.match(
    homeSource,
    /<\/section>\s*<AppDownloadSection \/>\s*\{\/\* 老师简介 teaser \*\/}/,
  )
  assert.ok(
    homeSource.indexOf('<AppDownloadSection />') < homeSource.indexOf('home.teacherTeaser'),
    'App 下载区需要位于老师简介之前',
  )
})

test('adds App download entry points to both navigation menus and Hero', () => {
  for (const collection of [siteConfig.navigation.main, siteConfig.navigation.drawer]) {
    assert.ok(collection.some((item) => (
      item.label === '下载 App'
      && item.to === '/#download-app'
      && item.type === 'hash'
    )))
  }

  assert.ok(siteConfig.home.hero.actions.some((action) => (
    action.label === '下载 App'
    && action.to === '#download-app'
    && action.type === 'anchor'
  )))
})

test('defines complete editable App download copy without release metadata', () => {
  const config = siteConfig.home.appDownload
  assert.ok(config)

  for (const key of [
    'eyebrow',
    'title',
    'lead',
    'androidButtonText',
    'iosComingSoonText',
    'unavailableText',
    'retryText',
  ]) {
    assert.equal(typeof config[key], 'string', `${key} 需要提供可编辑默认文案`)
    assert.ok(config[key].length > 0, `${key} 默认文案不能为空`)
  }

  assert.ok(Array.isArray(config.features) && config.features.length > 0)
  assert.ok(Array.isArray(config.installSteps) && config.installSteps.length > 0)

  for (const runtimeKey of [
    'versionName',
    'versionCode',
    'publishedAt',
    'fileSize',
    'sha256',
    'releaseNotes',
  ]) {
    assert.equal(Object.hasOwn(config, runtimeKey), false)
  }
})
