import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { mapPublishedClassroomItems, resolveClassroomItems } from './classroomCourseware.js'

assert.deepEqual(mapPublishedClassroomItems([]), [])
assert.deepEqual(mapPublishedClassroomItems([
  {
    id: 7,
    title: '关系沟通入门',
    description: '从觉察开始练习',
    coverUrl: 'https://cdn.example.test/7.jpg',
    contentType: 'video',
    durationSeconds: 125,
  },
]), [{
  id: '7',
  title: '关系沟通入门',
  description: '从觉察开始练习',
  cover: 'https://cdn.example.test/7.jpg',
  badge: '视频课件',
  duration: '约 2 分钟',
  materialTypes: ['视频'],
  bullets: [],
  url: '',
  classroomId: '7',
}])

const bundled = JSON.parse(await readFile(new URL('../data/studioVideos.json', import.meta.url), 'utf8'))
const fallback = resolveClassroomItems([], bundled, { allowFallback: true })
assert.equal(fallback.usedFallback, true, 'empty classroom responses should expose the bundled catalogue in devtools')
assert.equal(fallback.items.length, bundled.length)
assert.ok(mapPublishedClassroomItems(fallback.items).length > 0, 'bundled studio videos should map to visible learning cards')

const failedFallback = resolveClassroomItems(undefined, bundled, { allowFallback: true })
assert.equal(failedFallback.usedFallback, true, 'failed classroom responses should expose the bundled catalogue in devtools')
assert.equal(mapPublishedClassroomItems(failedFallback.items).length, bundled.length, 'bundled classroom items should map to visible learning cards')

const remote = [{ id: 99, title: '后台已发布日常', contentType: 'video' }]
const preferred = resolveClassroomItems(remote, bundled, { allowFallback: true })
assert.equal(preferred.usedFallback, false, 'published classroom data should take precedence over bundled data')
assert.deepEqual(preferred.items, remote)

const productionEmpty = resolveClassroomItems([], bundled)
assert.deepEqual(productionEmpty, { items: [], usedFallback: false }, 'bundled fixtures must stay disabled outside the devtools fallback gate')

console.log('classroom courseware mapping tests passed')
