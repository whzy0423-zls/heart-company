import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const pagesDir = dirname(fileURLToPath(import.meta.url))
const homeSource = readFileSync(resolve(pagesDir, 'index/index.vue'), 'utf8')
const learnSource = readFileSync(resolve(pagesDir, 'learn/learn.vue'), 'utf8')
const resultSource = readFileSync(resolve(pagesDir, 'result/result.vue'), 'utf8')
const bookingSource = readFileSync(resolve(pagesDir, 'booking/booking.vue'), 'utf8')

for (const [name, source] of [
  ['首页', homeSource],
  ['学习页', learnSource],
  ['测试结果页', resultSource],
  ['预约完成页', bookingSource],
]) {
  assert.match(
    source,
    /normalizeMiniappLearn/,
    `${name}应读取统一的小程序学习页配置`,
  )
  assert.match(
    source,
    /classroomEnabled/,
    `${name}应消费视频课程总入口开关`,
  )
}

assert.match(
  resultSource,
  /v-if="classroomEnabled"\s+class="result-recommendations/,
  '测试结果页课堂推荐入口应受总开关控制',
)
assert.match(
  bookingSource,
  /v-if="classroomEnabled"[^>]*@click="continueClassroom"/,
  '预约完成页继续课堂入口应受总开关控制',
)

assert.match(
  homeSource,
  /<view\s+v-if="classroomEnabled"[^>]*class="[^"]*\bservice-entry--course\b[^"]*"[^>]*@tap="goCourse"/,
  '首页课程学习入口应受总开关控制',
)
assert.match(
  homeSource,
  /<section\s+v-if="classroomEnabled"\s+class="content-section"\s+aria-labelledby="course-heading">/,
  '首页推荐课程区应受总开关控制',
)
assert.match(
  learnSource,
  /v-if="classroomEnabled"[^>]*id="learn-tab-course"/,
  '学习页课程分类入口应受总开关控制',
)
assert.match(
  learnSource,
  /if\s*\(!classroomEnabled\.value[\s\S]*activeCategory\.value\s*=\s*['"]quote['"]/,
  '关闭入口后学习页应从课程分类回退到可见分类',
)

console.log('miniapp classroom entry visibility tests passed')
