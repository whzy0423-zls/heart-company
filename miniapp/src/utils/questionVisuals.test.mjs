import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

import { QUESTIONS } from '../data/enneagramGame.js'
import { QUESTION_VISUAL_CENTERS, questionVisualCenter } from './questionVisuals.js'

const allowedCenters = new Set(['head', 'heart', 'gut'])

assert.equal(
  QUESTION_VISUAL_CENTERS.length,
  QUESTIONS.length,
  'presentation mapping should define one visual center for every live question',
)
assert.deepEqual(
  Object.keys(QUESTION_VISUAL_CENTERS),
  Array.from({ length: QUESTIONS.length }, (_, index) => String(index)),
  'presentation mapping should cover every live question index without gaps',
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
assert.equal(questionVisualCenter(QUESTIONS.length), 'head', 'indices beyond the live questions should use the stable fallback')
assert.equal(questionVisualCenter(Number.NaN), 'head', 'invalid indices should use the stable fallback')

const source = readFileSync(new URL('./questionVisuals.js', import.meta.url), 'utf8')
assert.doesNotMatch(source, /QUESTIONS|enneagramGame|answers?|scores?|\.w\b/, 'visual mapping must stay independent from question answers and scoring')

console.log('question visual tests passed')
