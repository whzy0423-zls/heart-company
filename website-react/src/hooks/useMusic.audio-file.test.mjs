import assert from 'node:assert/strict'
import { existsSync, readFileSync, statSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const hookSource = readFileSync(resolve(__dirname, 'useMusic.js'), 'utf8')
const audioPath = resolve(__dirname, '../../public/assets/audio/b8083359_audio.m4a')

assert.ok(existsSync(audioPath), '官网音乐文件需要放在 public/assets/audio 下以便部署')
assert.ok(statSync(audioPath).size > 1024 * 1024, '官网音乐文件大小异常，请确认已复制完整 m4a 文件')
assert.match(
  hookSource,
  /const\s+MUSIC_SRC\s*=\s*['"]\/assets\/audio\/b8083359_audio\.m4a['"]/,
  'useMusic 需要引用项目内 m4a 音乐文件',
)
assert.match(
  hookSource,
  /new Audio\(MUSIC_SRC\)/,
  'useMusic 需要使用真实音频文件播放',
)
assert.match(
  hookSource,
  /addEventListener\(['"]site:pause-music['"]/,
  '播放视频时需要能通过全局事件关闭背景音乐',
)
assert.match(
  hookSource,
  /removeEventListener\(['"]site:pause-music['"]/,
  '背景音乐暂停事件监听需要在卸载时清理',
)
assert.doesNotMatch(
  hookSource,
  /AudioContext|createOscillator|createConvolver|createBiquadFilter/,
  'useMusic 不应再使用 WebAudio 生成式音乐',
)

console.log('music audio file tests passed')
