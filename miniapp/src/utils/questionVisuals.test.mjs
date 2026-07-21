import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { QUESTION_VISUAL_CENTERS, questionVisualCenter } from './questionVisuals.js'

const allowedCenters = new Set(['head', 'heart', 'gut'])

assert.equal(QUESTION_VISUAL_CENTERS.length, 12, 'presentation mapping should define exactly 12 question indices')
assert.deepEqual(
  Object.keys(QUESTION_VISUAL_CENTERS),
  Array.from({ length: 12 }, (_, index) => String(index)),
  'presentation mapping should cover indices 0 through 11 without gaps',
)
for (const center of QUESTION_VISUAL_CENTERS) {
  assert.ok(allowedCenters.has(center), `unexpected visual center: ${center}`)
}

for (let index = 0; index < QUESTION_VISUAL_CENTERS.length; index += 1) {
  assert.equal(questionVisualCenter(index), QUESTION_VISUAL_CENTERS[index], `index ${index} should be deterministic`)
  assert.equal(
    questionVisualCenter(index, { selectedAnswer: { w: { 9: 99 } }, score: { head: 999 } }),
    QUESTION_VISUAL_CENTERS[index],
    `index ${index} should not depend on answers or scoring`,
  )
}

assert.equal(questionVisualCenter(-1), 'head', 'negative indices should use the stable fallback')
assert.equal(questionVisualCenter(12), 'head', 'indices beyond the explicit mapping should use the stable fallback')
assert.equal(questionVisualCenter(Number.NaN), 'head', 'invalid indices should use the stable fallback')

const source = readFileSync(new URL('./questionVisuals.js', import.meta.url), 'utf8')
assert.doesNotMatch(source, /QUESTIONS|enneagramGame|answers?|scores?|\.w\b/, 'visual mapping must stay independent from question answers and scoring')

console.log('question visual tests passed')
