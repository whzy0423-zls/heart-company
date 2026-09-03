import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const root = resolve(new URL('.', import.meta.url).pathname, '..')
const css = readFileSync(resolve(root, 'index.css'), 'utf8')
const tokens = readFileSync(resolve(root, 'styles/tokens.css'), 'utf8')
const motion = readFileSync(resolve(root, 'styles/motion.css'), 'utf8')
const home = readFileSync(resolve(root, 'pages/Home.jsx'), 'utf8')
const provider = readFileSync(resolve(root, 'motion/MotionProvider.jsx'), 'utf8')
const scene = readFileSync(resolve(root, 'components/EnneagramScene.jsx'), 'utf8')
const story = readFileSync(resolve(root, 'components/MotionStory.jsx'), 'utf8')
const pointerField = readFileSync(resolve(root, 'motion/usePointerField.js'), 'utf8')
const packageJson = JSON.parse(readFileSync(resolve(root, '../package.json'), 'utf8'))

test('motion site exposes a premium motion system with reduced-motion fallback', () => {
  assert.match(css, /prefers-reduced-motion/, 'motion site needs an accessible reduced-motion mode')
  assert.match(`${css}\n${tokens}\n${motion}`, /--motion-duration|--ease-premium/, 'motion site needs shared motion tokens')
  assert.match(home, /EnneagramOrbit|MotionBackdrop/, 'home needs the branded motion primitives')
  assert.match(home, /data-motion-section|motion-section/, 'home needs named motion sections for scroll verification')
})

test('motion site uses the reference stack for cinematic interactions', () => {
  assert.ok(packageJson.dependencies.gsap, 'GSAP must be installed in the isolated motion site')
  assert.ok(packageJson.dependencies.lenis, 'Lenis must be installed in the isolated motion site')
  assert.match(provider, /ScrollTrigger/, 'GSAP ScrollTrigger must orchestrate section motion')
  assert.match(provider, /Lenis/, 'Lenis must provide smooth scrolling')
  assert.match(home, /EnneagramField/, 'home needs a business-specific particle field')
  assert.match(home, /data-magnetic/, 'home actions need magnetic interaction markers')
  assert.match(home, /data-parallax/, 'home media needs scroll parallax markers')
})

test('motion site includes a full-bleed Three.js enneagram scene', () => {
  assert.ok(packageJson.dependencies.three, 'Three.js must power the immersive hero scene')
  assert.match(home, /EnneagramScene/, 'home must render the full-bleed enneagram scene')
  assert.match(home, /motion-hero__matrix/, 'hero must include the MiMo-inspired repeating type matrix')
  assert.match(motion, /\.enneagram-scene/, 'scene needs stable full-bleed layout styles')
  assert.match(motion, /\.motion-hero__matrix/, 'type matrix needs dedicated visual styling')
})

test('mobile hero reserves the full business rail height below its copy', () => {
  assert.match(
    motion,
    /@media \(max-width: 560px\)[\s\S]*?\.motion-hero \{[^}]*padding:\s*0 0 72px;/,
    'mobile hero copy must clear the absolutely positioned 66px business rail',
  )
})

test('Three.js scene avoids the deprecated Clock runtime warning', () => {
  assert.doesNotMatch(scene, /\bClock\b/, 'scene timing should use the browser animation timestamp')
})

test('desktop floating controls clear the hero business rail', () => {
  assert.match(motion, /\.motion-shell \.music \{[^}]*bottom:\s*136px;/, 'desktop music control must sit above the hero rail')
  assert.match(motion, /\.motion-shell \.customer-service-fab \{[^}]*bottom:\s*196px;/, 'customer service must stack above the music control')
})

test('dark growth story exposes a pointer-following spotlight', () => {
  assert.match(story, /data-pointer-spotlight/, 'dark story must declare the interactive spotlight region')
  assert.match(story, /motion-story__spotlight/, 'dark story must render a dedicated light layer')
  assert.match(pointerField, /--spotlight-x/, 'pointer field must update the spotlight horizontal position')
  assert.match(pointerField, /--spotlight-y/, 'pointer field must update the spotlight vertical position')
  assert.match(pointerField, /pointerenter|pointermove/, 'spotlight must react to pointer interaction')
  assert.match(motion, /radial-gradient\([^)]*var\(--spotlight-x\)[^)]*var\(--spotlight-y\)/, 'spotlight must use pointer coordinates in its radial light')
  assert.match(motion, /\.motion-story[^}]*background:\s*#0[0-9a-f]{5}/i, 'spotlight section needs a near-black base')
})

console.log('motion contract test passed')
