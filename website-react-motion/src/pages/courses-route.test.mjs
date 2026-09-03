import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const appSource = readFileSync(resolve(__dirname, '../App.jsx'), 'utf8')
const configSource = readFileSync(resolve(__dirname, '../../../shared/site-config.json'), 'utf8')
const coursesSource = readFileSync(resolve(__dirname, 'Courses.jsx'), 'utf8')

assert.match(appSource, /const\s+Courses\s*=\s*lazy\(\(\)\s*=>\s*import\('\.\/pages\/Courses'\)\)/, '官网需要懒加载课程页面')
assert.match(appSource, /path="courses"/, '官网需要 /courses 课程路由')
assert.match(configSource, /"label":\s*"课程"[\s\S]*?"to":\s*"\/courses"[\s\S]*?"type":\s*"route"/, '课程导航需要跳转到 /courses 独立页面')
assert.doesNotMatch(configSource, /"label":\s*"课程"[\s\S]*?"to":\s*"\/#courses"/, '课程导航不应再跳回首页锚点')
assert.match(coursesSource, /siteConfig\.home\.courses/, '课程页面需要复用后台配置的课程数据')

console.log('courses route tests passed')
