import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('./learn.vue', import.meta.url), 'utf8')
const template = source.match(/<template>([\s\S]*?)<\/template>/)?.[1] || ''
const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1] || ''

assert.match(
  script,
  /const\s+TEACHER_FALLBACK\s*=\s*['"]\/static\/teacher\/portrait\.jpg['"]/
    ,
  'learning center should use the bundled teacher portrait when content has no usable avatar',
)
assert.match(script, /const\s+teacherPreviewImage\s*=\s*computed\(\(\)\s*=>[\s\S]*hero-portrait[\s\S]*TEACHER_PREVIEW_FALLBACK/, 'teacher avatar preview should resolve the original portrait when the thumbnail uses the wide hero composition')
assert.match(script, /function\s+previewTeacherAvatar\s*\(\s*\)\s*\{[\s\S]*previewImage\(teacherPreviewImage\.value\)/, 'teacher avatar should use the original source with the shared preview helper')
assert.match(script, /import\s+NxImagePreview\s+from\s+['"]\.\.\/\.\.\/components\/NxImagePreview\.vue['"]/, 'learning center should provide an in-app preview fallback')
assert.match(script, /import\s+\{\s*isWechatDevtools\s*\}\s+from\s+['"]\.\.\/\.\.\/utils\/imagePreview['"]/, 'learning center should detect the developer-tools preview runtime')
assert.match(script, /const\s+teacherPreviewVisible\s*=\s*ref\(false\)/, 'learning center should track its in-app teacher preview state')
assert.match(script, /function\s+previewTeacherAvatar\s*\(\s*\)\s*\{[\s\S]*isWechatDevtools\(\)[\s\S]*teacherPreviewVisible\.value\s*=\s*true/, 'developer tools should open the in-app preview instead of relying on native preview')
assert.match(script, /function\s+previewCourseCover\s*\(\s*courseKey\s*\)/, 'learning center should expose a course cover preview action')
assert.match(script, /previewCourseCover[\s\S]*previewImage\(current,\s*\{\s*urls\s*\}\)/, 'course cover preview should include the available course cover URLs')

const teacherAvatarButton = template.match(/<button\b(?=[^>]*class=["']teacher-card__avatar-action["'])(?=[^>]*@click=["']previewTeacherAvatar["'])[^>]*>[\s\S]*?<\/button>/)?.[0] || ''
assert.ok(teacherAvatarButton, 'teacher avatar should remain a tappable preview button')
const teacherAvatarImage = teacherAvatarButton.match(/<image\b[^>]*class=["']learn-teacher__image["'][^>]*>/)?.[0] || ''
assert.match(teacherAvatarImage, /mode=["']widthFix["']/, 'teacher avatar should fill the frame from the top while keeping the head visible')
// Use the full SFC source here because the page contains a nested
// `<template v-if>` block; a minimal non-greedy template extraction would
// otherwise stop before the page-level preview component.
assert.match(source, /<NxImagePreview\b[\s\S]*:visible=["']teacherPreviewVisible["'][\s\S]*:src=["']teacherPreviewImage["']/, 'learning center should render the original teacher image in a closable preview')

const featuredCover = template.match(/<image\b[^>]*class=["']featured-video__cover["'][^>]*>/)?.[0] || ''
assert.match(featuredCover, /@click\.stop=["']previewCourseCover\(featuredCourse\.key\)["']/, 'featured cover should preview without opening the course')

const courseCovers = template.match(/<image\b[^>]*class=["']course-row__cover["'][^>]*>/g) || []
assert.ok(courseCovers.length >= 1, 'course rows should render at least one cover image')
for (const cover of courseCovers) {
  assert.match(cover, /@click\.stop=["']previewCourseCover\(courseEntry\.key\)["']/, 'course row cover should preview without opening the course')
}

assert.match(source, /\.teacher-card__avatar-action\s*\{[^}]*padding:\s*0[^}]*border:\s*0/, 'teacher avatar button should remove native button chrome')
assert.match(source, /\.learn-teacher__image\s*\{[^}]*width:\s*88rpx[^}]*min-height:\s*110rpx[^}]*border-radius:\s*14rpx/, 'teacher avatar should fill the reserved portrait frame from the top')

console.log('learn avatar and cover preview tests passed')
