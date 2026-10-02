import assert from 'node:assert/strict'
import { buildRelationAnalysis } from './relationAnalysis.js'
import { PAIRS_1_3 } from './data/pairs-1-3.js'
import { PAIRS_4_9 } from './data/pairs-4-9.js'
import { buildRelationSources, COMMUNICATION_TOOLS } from './data/relationSources.js'

const pairs = { ...PAIRS_1_3, ...PAIRS_4_9 }
assert.equal(Object.keys(PAIRS_1_3).length, 24)
assert.equal(Object.keys(PAIRS_4_9).length, 21)
assert.equal(Object.keys(pairs).length, 45, 'every unique pairing has its own researched entry')
const bridges = new Set(), scenes = new Set()
for (let mine = 1; mine <= 9; mine++) for (let other = 1; other <= 9; other++) {
  const result = buildRelationAnalysis(mine, other)
  const reversed = buildRelationAnalysis(other, mine)
  const key = [mine, other].sort((a,b) => a-b).join('-')
  assert.deepEqual(result.pairInsight, pairs[key])
  assert.deepEqual(result.pairInsight, reversed.pairInsight, 'public pair material is independent of the recipient/order')
  assert.equal(result.needs[0].typeId, mine, 'personal roles still follow selection order')
  const pair = result.pairInsight
  for (const field of ['theme', 'bridge', 'friction']) assert.ok(typeof pair[field] === 'string' && pair[field].length > 5)
  for (const field of ['title','situation','tryThis','question']) assert.ok(pair.scene[field]?.length > 5, `${key} scene ${field}`)
  assert.ok(pair.bridge.length >= 35 && pair.friction.length >= 35)
  bridges.add(pair.bridge); scenes.add(pair.scene.situation)
  const source = result.sources.find(s => s.id === 'pair-theory')
  assert.equal(source.url, `https://www.enneagraminstitute.com/relationship-type-${Math.min(mine,other)}-with-type-${Math.max(mine,other)}/`)
  assert.deepEqual(result.sources, buildRelationSources(mine,other))
  for (const entry of result.sources) {
    const url = new URL(entry.url)
    assert.equal(url.protocol,'https:')
    assert.ok(['www.enneagraminstitute.com','www.gottman.com','doi.org'].includes(url.hostname))
    assert.equal(url.search,'')
    assert.ok(entry.title && entry.note && entry.publisher && entry.checkedAt)
  }
  for (const tool of result.communicationTools) assert.ok(result.sources.some(s => s.id === tool.sourceId))
  assert.doesNotMatch(JSON.stringify(pair), /undefined|NaN|匹配率|百分之|科学证实|天生一对|注定/)
}
assert.equal(bridges.size,45,'pair introductions must not fall back to the same center template')
assert.equal(scenes.size,45,'each pairing has a distinct practice scenario')
assert.equal(COMMUNICATION_TOOLS.length,4)
const mutated = buildRelationAnalysis(2,5)
mutated.pairInsight.scene.question = 'changed'
mutated.sources[0].url = 'changed'
mutated.communicationTools[0].text = 'changed'
assert.notEqual(buildRelationAnalysis(2,5).pairInsight.scene.question, 'changed')
assert.notEqual(buildRelationAnalysis(5,2).sources[0].url, 'changed')
assert.notEqual(buildRelationAnalysis(2,5).communicationTools[0].text, 'changed')
console.log('Relation sources: 45 researched pairs, 81 views, original scenes, citations and isolation passed')
