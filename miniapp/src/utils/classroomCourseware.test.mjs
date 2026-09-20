import assert from 'node:assert/strict'
import { mapPublishedClassroomItems } from './classroomCourseware.js'

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
