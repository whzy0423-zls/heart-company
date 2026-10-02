import assert from 'node:assert/strict'
import { buildRelationAnalysis as analyze } from '../pages/relation/relationAnalysis.js'
import { TYPES_INFO } from '../data/enneagramGame.js'

const narratives = new Set()
for (let mine = 1; mine <= 9; mine += 1) {
  for (let other = 1; other <= 9; other += 1) {
    const result = analyze(mine, other)
    const reverse = analyze(other, mine)
    narratives.add(result.bond + result.friction)
    assert.deepEqual(Object.keys(result).sort(), ['label', 'bond', 'friction', 'tip', 'myDrive', 'taDrive', 'needs', 'strengths', 'dialogue', 'practices', 'pairInsight', 'sources', 'communicationTools'].sort())
    for (const field of ['bond', 'friction', 'tip', 'myDrive', 'taDrive']) assert.ok(result[field].length > 12, `${mine}/${other} ${field}`)
    assert.equal(result.needs.length, 2)
    assert.deepEqual(result.needs.map(({ role, typeId, name }) => ({ role, typeId, name })), [
      { role: '我', typeId: mine, name: TYPES_INFO[mine].name },
      { role: 'TA', typeId: other, name: TYPES_INFO[other].name },
    ])
    for (const need of result.needs) {
      for (const field of ['need', 'support', 'stress']) assert.ok(need[field].length > 8)
    }
    assert.equal(result.strengths.length, 2)
    assert.equal(result.dialogue.length, 2)
    assert.equal(result.practices.length, 3)
    for (const item of [...result.strengths, ...result.practices]) {
      assert.ok(item.title.length > 3)
      assert.ok(item.text.length > 15)
    }
    // The same report is read by recipients of a share. Only direct example
    // quotations may use first/second person; narration identifies each type.
    const narration = [result.bond, result.friction, result.tip,
      ...[...result.strengths, ...result.practices].flatMap(({ title, text }) => [title, text]),
    ].join('\n').replace(/“[^”]*”/g, '')
    assert.doesNotMatch(narration, /你|TA|我们/, 'public narration must not assume the reader is either participant')
    if (mine !== other) {
      for (const field of ['bond', 'friction', 'tip']) {
        assert.ok(result[field].includes(`${mine}号一方`))
        assert.ok(result[field].includes(`${other}号一方`))
      }
      assert.ok(result.strengths[0].title.startsWith(`${mine}号一方`))
      assert.ok(result.strengths[1].title.startsWith(`${other}号一方`))
    }
    assert.deepEqual(result.dialogue.map((item) => item.role), ['我对 TA 说', 'TA 对我说'])
    assert.equal(result.dialogue[0].text, reverse.dialogue[1].text, 'swapped roles must preserve the actual speaker/recipient advice')
    assert.equal(result.dialogue[1].text, reverse.dialogue[0].text)
    assert.equal(result.needs[0].need, reverse.needs[1].need)
    assert.equal(result.needs[0].support, reverse.needs[1].support)
    assert.equal(result.practices[0].text, reverse.practices[0].text, 'center exercise should be symmetric')
    assert.doesNotMatch(JSON.stringify(result), /undefined|NaN|匹配率|契合度|%/)
    if (mine === other) {
      assert.equal(result.label, '同型照见')
      assert.notEqual(result.strengths[0].text, result.strengths[1].text, 'same-type strengths must not duplicate')
      assert.match(result.bond, /经历与表达方式仍会不同/)
    } else if (TYPES_INFO[mine].center === TYPES_INFO[other].center) {
      assert.equal(result.label, '同频共鸣')
    } else {
      assert.equal(result.label, '互补同行')
    }
  }
}
assert.equal(narratives.size, 81, 'all ordered combinations need type-specific narratives')

// Concrete recipient needs must survive composition, not just label swapping.
const twoFive = analyze(2, 5)
assert.match(twoFive.dialogue[0].text, /先想一想.*时间再聊/)
assert.match(twoFive.dialogue[1].text, /谢谢你.*帮你做什么/)
assert.match(twoFive.practices[1].text, /^2号一方可以提前说明.*5号一方可以问一件对方真正想被照顾/)
assert.match(twoFive.tip, /^支持5号一方.*2号一方也可以说清自己的需要/)
assert.match(analyze(8, 9).dialogue[0].text, /听你的偏好.*慢慢说/)
assert.match(analyze(8, 9).dialogue[1].text, /坦诚.*尊重你的选择/)
assert.notEqual(analyze(1, 2).practices[0].text, analyze(1, 5).practices[0].text)
assert.deepEqual(analyze('2', '5'), twoFive, 'route string IDs must work')
const invalid = [null, undefined, '', ' ', 0, 10, -1, 1.5, '01', '1.0', '1e0', '1x', ' 1 ', true, false, [], [1], {}, NaN, Infinity]
for (const value of invalid) {
  assert.equal(analyze(value, 1), null)
  assert.equal(analyze(1, value), null)
}
twoFive.needs[0].need = 'changed'
twoFive.strengths[0].text = 'changed'
assert.notEqual(analyze(2, 5).needs[0].need, 'changed', 'results must not expose shared mutable profile objects')
assert.notEqual(analyze(2, 5).strengths[0].text, 'changed')
console.log('relation analysis: 81 ordered pairs, role direction, same types and invalid inputs passed')
