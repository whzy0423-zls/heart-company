import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const source = await readFile(new URL('./profile.vue', import.meta.url), 'utf8')
const anonymousStart = source.indexOf('<view v-if="!logged"')
const signedInStart = source.indexOf('<template v-else>')

assert.ok(anonymousStart >= 0, 'profile should keep the logged-out hero')
assert.ok(signedInStart > anonymousStart, 'profile should keep the signed-in state after the logged-out hero')

const anonymousHero = source.slice(anonymousStart, signedInStart)
const signedInProfile = source.slice(signedInStart)

assert.match(
  anonymousHero,
  /<image\s+src="\/static\/wheel\.png"\s+mode="aspectFit"\s+aria-label="九型 Logo"[^>]*\/>/,
  'the logged-out hero should use the complete Nine-Type logo',
)
assert.doesNotMatch(
  anonymousHero,
  /class="profile-hero__mark"\s*>九<\/view>/,
  'the logged-out hero should no longer use the single-character placeholder',
)

assert.match(
  signedInProfile,
  /<image\s+v-if="user && user\.avatar && !userAvatarFailed"[^>]*:src="user\.avatar"[^>]*@error="onUserAvatarError"[^>]*\/>/,
  'a valid signed-in user avatar should remain visible until loading fails',
)
assert.match(
  signedInProfile,
  /<image\s+v-else\s+src="\/static\/wheel\.png"\s+mode="aspectFit"\s+aria-label="九型 Logo"[^>]*\/>/,
  'a missing or failed signed-in avatar should fall back to the same Nine-Type logo',
)
assert.equal(
  source.match(/src="\/static\/wheel\.png"/g)?.length,
  2,
  'the logged-out and signed-in fallback states should share the same logo asset',
)

assert.match(
  source,
  /\.user__avatar\s*\{[^}]*width:\s*104rpx;[^}]*height:\s*104rpx;[^}]*border-radius:\s*34rpx;[^}]*border:\s*3rpx solid rgba\(223, 188, 127, \.72\);/,
  'the signed-in avatar should retain its size, rounded corners, and gold border',
)
assert.match(
  source,
  /\.profile-logo\s*\{[^}]*box-sizing:\s*border-box;[^}]*background:\s*var\(--nx-brand-900\);/,
  'logo fallbacks should remain contained on the deep-blue brand background',
)
const profileLogoStyleStart = source.indexOf('.profile-logo')
const avatarPlaceholderStyleStart = source.indexOf('.user__avatar--ph')
assert.ok(
  profileLogoStyleStart > avatarPlaceholderStyleStart,
  'the logo fallback background should be declared after the legacy placeholder background',
)

console.log('profile logo tests passed')
