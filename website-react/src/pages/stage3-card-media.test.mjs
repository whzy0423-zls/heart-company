import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const stagesSource = readFileSync(resolve(__dirname, 'Stages.jsx'), 'utf8')
const cssSource = readFileSync(resolve(__dirname, '../index.css'), 'utf8')

const stage3Card = stagesSource.match(/第 03 阶段[\s\S]*?to="\/stage3"/)?.[0] || ''

assert.match(
  stage3Card,
  /className="figure[^"]*stage3-media[^"]*"/,
  '第三阶段卡片需要使用组合图片区承载 logo 和合照',
)

assert.match(
  stage3Card,
  /\/assets\/wheel\.png[\s\S]*\/assets\/teacher-mentor\.jpg/,
  '第三阶段卡片需要在 logo 下方加入韩常青与陈伟志博士合照',
)

assert.match(
  stage3Card,
  /alt="韩常青与陈伟志博士合影"/,
  '合照需要有明确的图片说明',
)

assert.match(
  cssSource,
  /\.stage3-media\s*\{[\s\S]*display:\s*grid;[\s\S]*grid-template-columns:\s*minmax\(92px,\s*\.46fr\)\s+minmax\(0,\s*1fr\);[\s\S]*aspect-ratio:\s*6\s*\/\s*5;[\s\S]*isolation:\s*isolate;/,
  '第三阶段组合图片区桌面端需要左右分栏完整展示 logo 和合照，避免被折叠裁切',
)

assert.match(
  cssSource,
  /\.stage3-media__mentor\s*\{[\s\S]*display:\s*grid;[\s\S]*place-items:\s*center;/,
  '第三阶段合照容器需要居中展示完整图片',
)

assert.match(
  cssSource,
  /\.stage3-media__logo\s+img\s*\{[\s\S]*width:\s*min\(84%,\s*150px\);[\s\S]*height:\s*auto;/,
  '第三阶段 logo 需要完整展示，不能被容器裁切',
)

assert.match(
  cssSource,
  /@media\s*\(max-width:\s*920px\)\s*\{[\s\S]*\.stage3-media\s*\{[\s\S]*grid-template-columns:\s*minmax\(72px,\s*\.28fr\)\s+minmax\(0,\s*1fr\);[\s\S]*align-items:\s*center;[\s\S]*aspect-ratio:\s*auto;/,
  '第三阶段组合图片区窄屏需要用小 logo 徽章加原图比例合照，减少 contain 造成的大块留白',
)

assert.match(
  cssSource,
  /@media\s*\(max-width:\s*920px\)\s*\{[\s\S]*\.stage3-media__logo\s*\{[\s\S]*width:\s*clamp\(66px,\s*18vw,\s*128px\);[\s\S]*aspect-ratio:\s*1;[\s\S]*justify-self:\s*center;/,
  '第三阶段移动端 logo 需要缩成独立徽章，不能撑满整条图片高度',
)

assert.match(
  cssSource,
  /@media\s*\(max-width:\s*920px\)\s*\{[\s\S]*\.stage3-media__mentor\s*\{[\s\S]*aspect-ratio:\s*1200\s*\/\s*748;[\s\S]*border-width:\s*2px;/,
  '第三阶段移动端合照需要按原图比例占位，避免上下灰色留白',
)

assert.match(
  cssSource,
  /\.stage3-media__mentor\s+img\s*\{[\s\S]*object-fit:\s*contain;/,
  '合照需要使用 contain 完整展示，不能裁切老师图片',
)

console.log('stage3 card media tests passed')
