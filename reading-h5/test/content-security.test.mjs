import assert from 'node:assert/strict'
import test from 'node:test'

import * as markdownUtils from '../src/utils/markdown.js'
import { resolveApiMediaUrl } from '../src/utils/mediaUrl.js'

const { renderMarkdown } = markdownUtils

test('renderMarkdown removes raw HTML and unsafe markdown URLs', () => {
  const html = renderMarkdown(`
# Safe heading

<img src=x onerror=alert(1)>
<script>alert(1)</script>

[bad link](javascript:alert(1))
[safe link](https://example.com/guide)
[http link](http://example.com/guide)
![bad image](javascript:alert(1))
![bad protocol relative image](//evil.example/track.png)
![safe image](/api/files/cover.png)
`)

  assert.match(html, /<h1>Safe heading<\/h1>/)
  assert.match(html, /<a href="https:\/\/example\.com\/guide">safe link<\/a>/)
  assert.match(html, /<img src="\/api\/files\/cover\.png" alt="safe image">/)
  assert.doesNotMatch(html, /href="http:\/\/example\.com\/guide"/i)
  assert.doesNotMatch(html, /<script/i)
  assert.doesNotMatch(html, /onerror/i)
  assert.doesNotMatch(html, /javascript:/i)
  assert.doesNotMatch(html, /evil\.example/i)
})

test('renderMarkdown only allows https or relative markdown images', () => {
  const html = renderMarkdown(`
![http image](http://cdn.example.com/cover.png)
![https image](https://cdn.example.com/cover.png)
![relative image](/api/files/cover.png)
`)

  assert.match(html, /<img src="https:\/\/cdn\.example\.com\/cover\.png" alt="https image">/)
  assert.match(html, /<img src="\/api\/files\/cover\.png" alt="relative image">/)
  assert.doesNotMatch(html, /http:\/\/cdn\.example\.com/i)
})

test('sanitizeRenderedHtml removes event handlers and unsafe attributes', () => {
  assert.equal(typeof markdownUtils.sanitizeRenderedHtml, 'function')

  const html = markdownUtils.sanitizeRenderedHtml(`
    <p onclick="alert(1)">hello</p>
    <img src="x" onerror="alert(1)">
    <a href="javascript:alert(1)">bad</a>
    <a href="http://example.com/guide">http</a>
    <a href="https://example.com/guide">safe</a>
  `)

  assert.match(html, /hello/)
  assert.match(html, /href="https:\/\/example\.com\/guide"/)
  assert.doesNotMatch(html, /href="http:\/\/example\.com\/guide"/i)
  assert.doesNotMatch(html, /onclick/i)
  assert.doesNotMatch(html, /onerror/i)
  assert.doesNotMatch(html, /javascript:/i)
})

test('resolveApiMediaUrl uses the API origin for relative media paths', () => {
  assert.equal(
    resolveApiMediaUrl('/uploads/covers/a.jpg', 'https://api.example.com/api/'),
    'https://api.example.com/uploads/covers/a.jpg',
  )
  assert.equal(
    resolveApiMediaUrl('uploads/covers/a.jpg', 'https://api.example.com/api'),
    'https://api.example.com/uploads/covers/a.jpg',
  )
  assert.equal(
    resolveApiMediaUrl('uploads/covers/a.jpg', 'https://api.example.com/api/'),
    'https://api.example.com/uploads/covers/a.jpg',
  )
  assert.equal(
    resolveApiMediaUrl('/api/files/a.mp3', 'https://api.example.com/api'),
    'https://api.example.com/api/files/a.mp3',
  )
})

test('resolveApiMediaUrl leaves absolute and same-origin paths unchanged', () => {
  assert.equal(
    resolveApiMediaUrl('https://cdn.example.com/a.jpg', 'https://api.example.com/api'),
    'https://cdn.example.com/a.jpg',
  )
  assert.equal(
    resolveApiMediaUrl('/uploads/covers/a.jpg', '/api'),
    '/uploads/covers/a.jpg',
  )
})

test('resolveApiMediaUrl rejects protocol-relative media paths', () => {
  assert.equal(
    resolveApiMediaUrl('//evil.example/a.jpg', 'https://api.example.com/api'),
    '',
  )
})

test('resolveApiMediaUrl rejects unsafe absolute media schemes', () => {
  for (const value of [
    'http://cdn.example.com/a.jpg',
    'javascript:alert(1)',
    'data:image/svg+xml,<svg onload=alert(1)>',
    'file:///etc/passwd',
    'ftp://cdn.example.com/a.jpg',
  ]) {
    assert.equal(resolveApiMediaUrl(value, 'https://api.example.com/api'), '')
  }
})
