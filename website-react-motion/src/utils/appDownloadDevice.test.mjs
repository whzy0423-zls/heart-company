import assert from 'node:assert/strict'
import test from 'node:test'

import {
  detectAppDownloadDevice,
  formatFileSizeMiB,
  formatLocalDate,
  formatSHA256,
} from './appDownloadDevice.js'

test('detects Android user agents', () => {
  assert.equal(detectAppDownloadDevice({
    userAgent: 'Mozilla/5.0 (Linux; Android 15; Pixel 9 Pro)',
    platform: 'Linux armv8l',
    maxTouchPoints: 5,
  }), 'android')
})

test('detects traditional iPhone, iPad, and iPod user agents', () => {
  for (const userAgent of [
    'Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X)',
    'Mozilla/5.0 (iPad; CPU OS 17_7 like Mac OS X)',
    'Mozilla/5.0 (iPod touch; CPU iPhone OS 15_7 like Mac OS X)',
  ]) {
    assert.equal(detectAppDownloadDevice({ userAgent, platform: '', maxTouchPoints: 1 }), 'ios')
  }
})

test('detects iPadOS desktop mode from MacIntel touch capability', () => {
  assert.equal(detectAppDownloadDevice({
    userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15) AppleWebKit/605.1.15',
    platform: 'MacIntel',
    maxTouchPoints: 5,
  }), 'ios')
})

test('classifies desktop browsers as desktop', () => {
  assert.equal(detectAppDownloadDevice({
    userAgent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)',
    platform: 'Win32',
    maxTouchPoints: 0,
  }), 'desktop')
})

test('falls back unknown device data to desktop', () => {
  assert.equal(detectAppDownloadDevice({ userAgent: '', platform: '', maxTouchPoints: 0 }), 'desktop')
  assert.equal(detectAppDownloadDevice({ userAgent: null, platform: undefined, maxTouchPoints: null }), 'desktop')
})

test('formats byte sizes as MiB', () => {
  assert.equal(formatFileSizeMiB(0), '0.0 MiB')
  assert.equal(formatFileSizeMiB(15 * 1024 * 1024), '15.0 MiB')
  assert.equal(formatFileSizeMiB(15.25 * 1024 * 1024), '15.3 MiB')
  assert.equal(formatFileSizeMiB(null), '—')
})

test('formats release timestamps in the local timezone', () => {
  const timestamp = '2026-07-20T08:30:00Z'
  const expected = new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(timestamp))

  assert.equal(formatLocalDate(timestamp), expected)
  assert.equal(formatLocalDate('not-a-date'), '—')
})

test('groups SHA-256 digests into wrapping-safe chunks', () => {
  const digest = '0123456789abcdef'.repeat(4)
  assert.equal(
    formatSHA256(digest),
    '01234567 89abcdef 01234567 89abcdef 01234567 89abcdef 01234567 89abcdef',
  )
  assert.equal(formatSHA256(''), '—')
})
