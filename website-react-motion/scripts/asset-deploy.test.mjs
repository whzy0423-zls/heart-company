import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const pruneSource = readFileSync(resolve(__dirname, 'prune-dist-uploads.mjs'), 'utf8')
const dockerignoreSource = readFileSync(resolve(__dirname, '../../.dockerignore'), 'utf8')
const packageSource = readFileSync(resolve(__dirname, '../package.json'), 'utf8')

assert.doesNotMatch(
  pruneSource,
  /dist\/assets\/(videos|audio)/,
  '官网生产构建默认不能删除视频/音频资源，否则同源 /assets 会 404',
)

assert.doesNotMatch(
  dockerignoreSource,
  /^website-react\/public\/assets\/(videos|audio)\/$/m,
  'Docker 构建上下文不能排除官网 public 音视频资源，否则镜像内没有默认媒体资源',
)

assert.match(
  packageSource,
  /"test"\s*:\s*"find src scripts -name '\*\.test\.mjs' -print0 \| xargs -0 node --test"/,
  'website-react 需要 npm test 覆盖 src 和 scripts 下的 Node 回归测试',
)

console.log('website asset deployment tests passed')
