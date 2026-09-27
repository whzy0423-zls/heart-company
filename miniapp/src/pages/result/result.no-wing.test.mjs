import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const resultSource = fs.readFileSync(path.join(here, 'result.vue'), 'utf8')
const dataSource = fs.readFileSync(path.join(here, '../../data/enneagramGame.js'), 'utf8')
const enneagramSource = fs.readFileSync(path.join(here, '../../utils/enneagram.js'), 'utf8')

for (const retired of ['isWing', 'const wing', '侧翼', '翼型']) {
  assert.equal(resultSource.includes(retired), false, `result page still contains ${retired}`)
}
assert.equal(resultSource.includes('副型能量'), true, 'result page must label the second type as 副型能量')
assert.equal(resultSource.includes('你的副型倾向'), true, 'result page must label the second type as 副型倾向')

for (const retired of ['wings:', '侧翼', '翼型']) {
  assert.equal(dataSource.includes(retired), false, `enneagram metadata still contains ${retired}`)
}
assert.equal(enneagramSource.includes('isWing'), false, 'enneagram utility still exposes isWing')

console.log('result no-wing source contract passed')
