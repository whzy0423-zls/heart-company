import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const sources = await Promise.all([
  readFile(new URL('./index/index.vue', import.meta.url), 'utf8'),
  readFile(new URL('./booking/booking.vue', import.meta.url), 'utf8'),
  readFile(new URL('./course-detail/course-detail.vue', import.meta.url), 'utf8'),
  readFile(new URL('../utils/teacherCourseware.js', import.meta.url), 'utf8'),
])
const [home, booking, courseDetail, teacherCourseware] = sources

assert.match(home, /import\s+\{\s*previewImage\s*\}\s+from\s+['"]\.\.\/\.\.\/utils\/imagePreview(?:\.js)?['"]/, 'home should use the shared image preview helper')
assert.match(home, /function\s+previewPortrait\s*\(\s*\)\s*\{[\s\S]*previewImage\(/, 'home should expose a portrait preview action')
assert.match(home, /class=["']mentor-portrait-action["'][^>]*@click(?:\.stop)?=["']previewPortrait["']/, 'home portrait should have a dedicated preview tap target')
assert.match(home, /mentor-portrait-action\s*\{[^}]*z-index:\s*1/, 'home portrait preview target should sit above the shade')

assert.match(booking, /import\s+\{\s*previewImage\s*\}\s+from\s+['"]\.\.\/\.\.\/utils\/imagePreview(?:\.js)?['"]/, 'booking should use the shared image preview helper')
assert.match(booking, /function\s+previewTeacherAvatar\s*\(\s*\)\s*\{[\s\S]*previewImage\(/, 'booking should expose a teacher avatar preview action')
assert.match(booking, /class=["']teacher-avatar-action["'][^>]*@click\.stop=["']previewTeacherAvatar["']/, 'booking teacher avatar should have a dedicated preview tap target')
assert.match(booking, /const\s+teacherAvatar\s*=\s*computed\(/, 'booking should resolve a real teacher portrait before rendering')

assert.match(courseDetail, /import\s+\{\s*previewImage\s*\}\s+from\s+['"]\.\.\/\.\.\/utils\/imagePreview(?:\.js)?['"]/, 'course detail should use the shared image preview helper')
assert.match(courseDetail, /function\s+previewTeacherAvatar\s*\(\s*\)\s*\{[\s\S]*previewImage\(/, 'course detail should expose a teacher avatar preview action')
assert.match(courseDetail, /class=["']teacher-avatar-action["'][^>]*@click\.stop=["']previewTeacherAvatar["']/, 'course detail teacher avatar should have a dedicated preview tap target')
assert.match(courseDetail, /const\s+teacherAvatar\s*=\s*computed\(/, 'course detail should resolve a real teacher portrait before rendering')

assert.match(teacherCourseware, /avatar:\s*['"]\/static\/teacher\/portrait\.jpg['"]/, 'default teacher data should use the bundled real portrait')

console.log('teacher avatar preview coverage tests passed')
