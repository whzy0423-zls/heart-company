import assert from 'node:assert/strict'
import test from 'node:test'

import { normalizeAssetBase } from './assets.js'

test('normalizeAssetBase only allows https or same-origin asset bases', () => {
  assert.equal(normalizeAssetBase('https://cdn.example.com/assets/'), 'https://cdn.example.com/assets')
  assert.equal(normalizeAssetBase('/static/assets/'), '/static/assets')
  assert.equal(normalizeAssetBase(''), '')

  for (const value of [
    'http://cdn.example.com/assets',
    '//evil.example/assets',
    'javascript:alert(1)',
    'data:text/html,<script>alert(1)</script>',
  ]) {
    assert.equal(normalizeAssetBase(value), '')
  }
})
