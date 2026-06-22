import assert from 'node:assert/strict'
import { copyFile, mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-profile-'))
const modulePath = join(dir, 'wechatProfile.mjs')
await copyFile(new URL('./wechatProfile.js', import.meta.url), modulePath)
const { normalizeWechatProfile, hasProfilePayload } = await import(`file://${modulePath}`)

assert.deepEqual(
  normalizeWechatProfile({
    nickName: '  九型用户  ',
    avatarUrl: 'https://cdn.example.com/avatar.png',
    gender: 1,
  }),
  {
    nickname: '九型用户',
    avatar: 'https://cdn.example.com/avatar.png',
    gender: 'male',
  },
)

assert.deepEqual(
  normalizeWechatProfile({ nickname: '小九', avatar: 'https://cdn.example.com/a.png', gender: 'female' }),
  { nickname: '小九', avatar: 'https://cdn.example.com/a.png', gender: 'female' },
)

assert.deepEqual(normalizeWechatProfile({ nickName: '', avatarUrl: '', gender: 0 }), {})
assert.equal(hasProfilePayload({}), false)
assert.equal(hasProfilePayload({ nickname: '小九' }), true)

console.log('wechat profile tests passed')
await rm(dir, { force: true, recursive: true })
