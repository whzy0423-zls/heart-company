import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const stagesSource = readFileSync(resolve(__dirname, 'Stages.jsx'), 'utf8')
const cssSource = readFileSync(resolve(__dirname, '../index.css'), 'utf8')

const stageCards = stagesSource.match(/className="card stage-card"/g) || []
const stageLinks = stagesSource.match(/className="stage-card__link"/g) || []
const stageMedia = stagesSource.match(/className="figure stage-card__media/g) || []

assert.equal(stageCards.length, 3, '三阶段卡片需要统一使用 stage-card 结构')
assert.equal(stageLinks.length, 3, '三阶段卡片底部入口需要统一使用 stage-card__link')
assert.equal(stageMedia.length, 3, '三阶段卡片图片区需要统一使用 stage-card__media')

assert.match(
  cssSource,
  /\.stage-card\s*\{[\s\S]*display:\s*flex;[\s\S]*flex-direction:\s*column;[\s\S]*min-height:\s*100%;/,
  '阶段卡片需要纵向 flex，给底部入口提供对齐基础',
)

assert.match(
  cssSource,
  /\.stage-card__body\s*\{[\s\S]*min-height:\s*4\.9em;/,
  '阶段卡片正文需要稳定占位，避免入口按钮上下漂',
)

assert.match(
  cssSource,
  /\.stage-card__link\s*\{[\s\S]*margin-top:\s*auto;/,
  '阶段卡片底部入口需要用 margin-top:auto 贴到底部，实现同一水平线',
)

assert.match(
  cssSource,
  /\.stage-card__media\s*\{[\s\S]*aspect-ratio:\s*6\s*\/\s*5;[\s\S]*margin:\s*18px\s+0\s+16px;/,
  '阶段卡片图片区需要统一比例和间距，让三张卡片视觉节奏一致',
)

console.log('stage cards layout tests passed')
