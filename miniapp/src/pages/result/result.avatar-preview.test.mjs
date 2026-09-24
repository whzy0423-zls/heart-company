import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('./result.vue', import.meta.url), 'utf8')
const template = source.match(/<template>([\s\S]*?)<\/template>/)?.[1] || ''
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1] || ''

assert.match(script, /import\s+\{\s*previewImage\s*\}\s+from\s+['"]\.\.\/\.\.\/utils\/imagePreview(?:\.js)?['"]/, 'result should use the shared image preview helper')
assert.match(script, /function\s+previewResultAvatar\s*\(\s*\)/, 'result should expose a preview action for the main type avatar')
assert.match(script, /previewResultAvatar[\s\S]*previewImage\(/, 'result avatar preview should call the shared helper')
assert.match(
  template,
  /<button\b(?=[^>]*class="result-hero__avatar-action")(?=[^>]*@click="previewResultAvatar\(\)")[^>]*>[\s\S]*?<image\b(?=[^>]*class="result-hero__avatar")/,
  'result avatar should be inside an accessible preview button',
)
assert.match(template, /class="result-hero__avatar-fallback"/, 'avatar failures should retain their fallback')

console.log('result avatar preview tests passed')
