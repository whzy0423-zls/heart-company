import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const stage2Source = readFileSync(resolve(__dirname, 'Stage2.jsx'), 'utf8')
const stagesSource = readFileSync(resolve(__dirname, 'Stages.jsx'), 'utf8')

assert.match(
  stage2Source,
  /\/assets\/types-map\.svg/,
  '第二阶段详情页需要使用带型号名称的九型人格类型图',
)

assert.doesNotMatch(
  stage2Source,
  /\/assets\/enneagram\.svg/,
  '第二阶段详情页不应再使用纯编号九型环图',
)

assert.match(
  stagesSource,
  /第 02 阶段[\s\S]*\/assets\/types-map\.svg/,
  '三阶段总览里的第二阶段卡片需要使用带型号名称的九型人格类型图',
)

console.log('stage2 image replacement tests passed')
