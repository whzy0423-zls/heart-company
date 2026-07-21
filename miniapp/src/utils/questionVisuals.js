// Presentation-only sequence for the first twelve editorial quiz frames.
export const QUESTION_VISUAL_CENTERS = Object.freeze([
  'heart',
  'head',
  'heart',
  'gut',
  'head',
  'heart',
  'gut',
  'heart',
  'head',
  'gut',
  'gut',
  'head',
])

const FALLBACK_CENTER = 'head'

export function questionVisualCenter(index) {
  return Number.isInteger(index) && index >= 0
    ? QUESTION_VISUAL_CENTERS[index] || FALLBACK_CENTER
    : FALLBACK_CENTER
}
