import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('./teacher.vue', import.meta.url), 'utf8')
const template = source.match(/<template>([\s\S]*?)<\/template>/)?.[1] || ''
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1] || ''

assert.match(script, /import\s+\{\s*previewImage\s*\}\s+from\s+['"]\.\.\/\.\.\/utils\/imagePreview(?:\.js)?['"]/, 'teacher page should use the shared image preview helper')
assert.match(script, /teacher-poster[^\n]*STUDIO_TEACHER\.avatar|STUDIO_TEACHER\.avatar[^\n]*teacher-poster/, 'teacher poster assets should use the bundled portrait fallback')
assert.match(script, /function\s+preview\s*\(\s*\)\s*\{[\s\S]*previewImage\(portrait\.value\)/, 'teacher portrait should use the shared preview helper')
assert.match(script, /function\s+previewMentorPhoto\s*\(\s*\)\s*\{[\s\S]*previewImage\(/, 'mentor photo should expose a preview action')

assert.match(template, /<image\b[^>]*class=["']portrait-image["'][^>]*@error=["']imageFailed\s*=\s*true["']/, 'teacher portrait should use an explicit image class and failure state')
assert.match(template, /<image\b[^>]*class=["']portrait-image["'][^>]*mode=["']aspectFit["']/, 'teacher portrait should preserve the full vertical source image')
assert.match(template, /<image\b[^>]*class=["']mentor-photo["'][^>]*@click=["']previewMentorPhoto["']/, 'mentor photo should be tappable for preview')
assert.match(source, /\.portrait-button\s*\{[^}]*display:\s*flex[^}]*overflow:\s*hidden/, 'portrait button should clip the image to its rounded frame')
assert.match(source, /\.portrait-image\s*\{[^}]*display:\s*block[^}]*width:\s*246rpx[^}]*height:\s*420rpx/, 'portrait image should fill the vertical frame')
assert.match(source, /\.portrait-button\s*\{[^}]*width:\s*246rpx[^}]*height:\s*420rpx[^}]*border-radius:\s*28rpx/, 'portrait frame should be a rounded vertical rectangle')

console.log('teacher avatar and photo preview tests passed')
