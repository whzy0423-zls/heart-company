import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const pages = JSON.parse(readFileSync(new URL('../pages.json', import.meta.url), 'utf8'))
const paths = new Set(pages.pages.map((item) => item.path))
for (const path of [
  'pages/classroom/classroom',
  'pages/classroom-detail/classroom-detail',
  'pages/booking-records/booking-records',
  'pages/booking-detail/booking-detail',
  'pages/profile-edit/profile-edit',
]) {
  assert.ok(paths.has(path), `${path} must be registered for production navigation`)
}

const learn = readFileSync(new URL('./learn/learn.vue', import.meta.url), 'utf8')
assert.match(learn, /classroomContentRoute/, 'learning page should use the canonical classroom content route')
assert.match(learn, /@tap="openPublishedCourse\(courseEntry\.item\)"/, 'published course cards should open their video/audio detail')
assert.match(learn, /@tap="openPublishedMaterial\(material\)"/, 'published material rows should open their video/audio detail')
assert.match(learn, /视频、音频与配套资料/, 'material panel should explain its purpose')

const testPage = readFileSync(new URL('./test/test.vue', import.meta.url), 'utf8')
assert.match(testPage, /center-\$\{currentVisualCenter\.value\}\.png/, 'question illustrations should use broadly compatible PNG assets')
assert.match(testPage, /\.quiz__visual\s*\{[\s\S]*?height:\s*174rpx/, 'question illustration must have an explicit height on real WeChat devices')

console.log('experience regression tests passed')
