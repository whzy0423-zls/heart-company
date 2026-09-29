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
assert.match(script, /function\s+previewTeacherAvatar\s*\(\s*\)\s*\{[\s\S]*previewImage\(teacherImage\.value\)/, 'teacher avatar should use the shared preview helper')
assert.match(script, /function\s+previewCourseCover\s*\(\s*courseKey\s*\)/, 'learning center should expose a course cover preview action')
assert.match(script, /previewCourseCover[\s\S]*previewImage\(current,\s*\{\s*urls\s*\}\)/, 'course cover preview should include the available course cover URLs')

const teacherAvatarButton = template.match(/<button\b(?=[^>]*class=["']teacher-card__avatar-action["'])(?=[^>]*@click=["']previewTeacherAvatar["'])[^>]*>[\s\S]*?<\/button>/)?.[0] || ''
assert.ok(teacherAvatarButton, 'teacher avatar should remain a tappable preview button')

const featuredCover = template.match(/<image\b[^>]*class=["']featured-video__cover["'][^>]*>/)?.[0] || ''
assert.match(featuredCover, /@click\.stop=["']previewCourseCover\(featuredCourse\.key\)["']/, 'featured cover should preview without opening the course')

const courseCovers = template.match(/<image\b[^>]*class=["']course-row__cover["'][^>]*>/g) || []
assert.ok(courseCovers.length >= 1, 'course rows should render at least one cover image')
for (const cover of courseCovers) {
  assert.match(cover, /@click\.stop=["']previewCourseCover\(courseEntry\.key\)["']/, 'course row cover should preview without opening the course')
}

assert.match(source, /\.teacher-card__avatar-action\s*\{[^}]*padding:\s*0[^}]*border:\s*0/, 'teacher avatar button should remove native button chrome')
assert.match(source, /\.learn-teacher__image\s*\{[^}]*width:\s*88rpx[^}]*height:\s*110rpx[^}]*border-radius:\s*14rpx/, 'teacher avatar should render inside the reserved portrait frame')

console.log('learn avatar and cover preview tests passed')
