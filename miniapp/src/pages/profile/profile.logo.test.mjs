import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('./profile.vue', import.meta.url), 'utf8')
assert.match(source, /<template v-if="!logged">/, 'profile should keep a clear logged-out state')
assert.match(source, /<template v-else>/, 'profile should keep a signed-in state')
assert.match(source, /微信一键登录|请在微信小程序内登录/, 'logged-out state should expose the login path')
assert.match(source, /class="user__avatar[^\"]*"/, 'signed-in state should render a user avatar slot')
assert.match(source, /userAvatarFailed|onUserAvatarError/, 'profile should track user image errors')
assert.match(source, /user__avatar--ph|login-card__icon/, 'profile should retain a non-empty local identity fallback')
assert.doesNotMatch(source, /#(?:172554|4338ca|7c3aed|4f46e5|3730a3|ddd6fe|ede9fe|f5f3ff|eef2ff)/i, 'profile should not keep the old violet theme')
console.log('profile logo tests passed')
