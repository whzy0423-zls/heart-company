import assert from 'node:assert/strict'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'

const packageJson = JSON.parse(readFileSync('package.json', 'utf8'))

assert.equal(packageJson.scripts['dev:h5'], 'uni -p h5')
assert.equal(packageJson.scripts['build:h5'], 'uni build -p h5')
assert.equal(
  packageJson.dependencies['@dcloudio/uni-h5'],
  packageJson.dependencies['@dcloudio/uni-app'],
)

const pagesConfig = readFileSync('src/pages.json', 'utf8')
assert.doesNotMatch(pagesConfig, /pages\/chat\/chat/, 'pages.json must not register the removed chat page')
assert.doesNotMatch(pagesConfig, /问 AI|AI 对话/, 'tabBar must not expose an AI chat entry')
assert.equal(statSync('src/pages/chat', { throwIfNoEntry: false }), undefined, 'removed chat page directory should stay deleted')

const h5Index = readFileSync('index.html', 'utf8')
assert.match(h5Index, /viewport-fit=cover/, 'H5 viewport meta should enable iOS safe-area env variables')

const appVue = readFileSync('src/App.vue', 'utf8')

const appleMobileStyle = readFileSync('src/styles/apple-mobile.css', 'utf8')
assert.match(appVue, /@import ['"]\.\/styles\/apple-mobile\.css['"];/, 'App.vue should import shared Apple/iOS mobile tokens')
for (const token of ['--nx-bg', '--nx-primary', '--nx-card', '--nx-radius', '--nx-safe-bottom']) {
  assert.match(appleMobileStyle, new RegExp(token), `apple-mobile.css should define ${token}`)
}
for (const token of [
  '--nx-page-bg',
  '--nx-surface',
  '--nx-surface-soft',
  '--nx-line',
  '--nx-blue',
  '--nx-purple',
  '--nx-pink',
  '--nx-teal',
  '--nx-green',
  '--nx-orange',
  '--nx-danger',
]) {
  assert.match(appleMobileStyle, new RegExp(`${token}\\s*:`), `apple-mobile.css should define ${token}`)
}
for (const className of ['.ios-page', '.ios-card', '.ios-button', '.ios-section', '.ios-safe-bottom']) {
  assert.match(appleMobileStyle, new RegExp(className.replace('.', '\\.') + '\\s*\\{'), `apple-mobile.css should define ${className}`)
}
for (const className of ['.nx-page-hero', '.nx-section-head', '.nx-panel', '.nx-state', '.nx-tag', '.nx-focusable']) {
  assert.match(appleMobileStyle, new RegExp(className.replace('.', '\\.') + '\\s*\\{'), `apple-mobile.css should define ${className}`)
}
assert.match(
  appleMobileStyle,
  /\.nx-focusable:focus\s*\{[^}]*outline\s*:\s*4rpx\s+solid\s+rgba\(\s*37\s*,\s*99\s*,\s*235\s*,\s*\.34\s*\)\s*;/,
  '.nx-focusable:focus should expose the planned visible outline',
)
assert.match(
  appleMobileStyle,
  /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{\s*\.nx-focusable\s*\{[^}]*animation\s*:\s*none\s*;/,
  'reduced-motion styles should disable .nx-focusable animation',
)
assert.match(
  appleMobileStyle,
  /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{\s*\.nx-focusable\s*\{[^}]*transition\s*:\s*none\s*;/,
  'reduced-motion styles should disable .nx-focusable transition',
)
assert.match(appleMobileStyle, /min-height:\s*88rpx/, 'Apple/iOS buttons should keep an 88rpx touch target')
assert.match(appleMobileStyle, /safe-area-inset-bottom/, 'Apple/iOS style tokens should reserve safe-area bottom')

function assertRootViewClasses(source, file, classNames) {
  const match = source.match(/<template>\s*<view\s+class=["']([^"']+)["']/)
  assert.ok(match, `${file} should render a root view with static classes`)
  const actual = match[1].split(/\s+/)
  for (const className of classNames) {
    assert.ok(actual.includes(className), `${file} root should include ${className}`)
  }
}

for (const file of ['src/pages/index/index.vue', 'src/pages/result/result.vue', 'src/pages/profile/profile.vue']) {
  const source = readFileSync(file, 'utf8')
  assert.match(source, /ios-page/, `${file} should opt into shared Apple/iOS page styling`)
  assert.match(source, /ios-card/, `${file} should opt into shared Apple/iOS card styling`)
}

for (const file of ['src/pages/relation/relation.vue', 'src/pages/test/test.vue', 'src/pages/learn/learn.vue', 'src/pages/booking/booking.vue']) {
  const source = readFileSync(file, 'utf8')
  assertRootViewClasses(source, file, ['page-stack', 'ios-page', 'ios-safe-bottom'])
}

const indexPage = readFileSync('src/pages/index/index.vue', 'utf8')
const homeOpeningViews = indexPage.match(/<view\b[^>]*>/g) || []

function staticClassTokens(tag) {
  const match = tag.match(/\sclass=["']([^"']*)["']/)
  return match ? match[1].trim().split(/\s+/).filter(Boolean) : []
}

function findHomeView(className) {
  return homeOpeningViews.find((tag) => staticClassTokens(tag).includes(className))
}

function bracedBody(match) {
  if (!match) return undefined
  const openingBrace = match.index + match[0].lastIndexOf('{')
  let depth = 0
  for (let index = openingBrace; index < indexPage.length; index += 1) {
    if (indexPage[index] === '{') depth += 1
    if (indexPage[index] !== '}') continue
    depth -= 1
    if (depth === 0) return indexPage.slice(openingBrace + 1, index)
  }
  return undefined
}

function functionBody(name) {
  return bracedBody(new RegExp(`function\\s+${name}\\s*\\(\\s*\\)\\s*\\{`).exec(indexPage))
}

function standaloneStyleDeclarations(className) {
  const escapedClassName = className.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return indexPage.match(new RegExp(`^[ \\t]*\\.${escapedClassName}\\s*\\{([^}]*)\\}`, 'm'))?.[1]
}

function assertKeyboardViewControl(tag, description, handler) {
  assert.match(tag, /\srole=["']button["']/, `${description} should use web button semantics`)
  assert.match(tag, /\saria-role=["']button["']/, `${description} should expose miniapp button semantics`)
  assert.match(tag, /\stabindex=["']0["']/, `${description} should participate in keyboard focus order`)
  assert.match(tag, new RegExp(`\\s@keydown\\.enter=["']${handler}["']`), `${description} should activate with Enter`)
  assert.match(tag, new RegExp(`\\s@keydown\\.space\\.prevent=["']${handler}["']`), `${description} should activate with Space without scrolling`)
}

function assertVisibleFocusStyle(className) {
  const escapedClassName = className.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const rules = [...indexPage.matchAll(/([^{}]+)\{([^{}]*)\}/g)]
  const focusRule = rules.find(([, selector]) => new RegExp(`\\.${escapedClassName}:focus(?:-visible)?(?:\\s|,|$)`).test(selector.trim()))
  assert.ok(focusRule, `.${className} should define a visible focus state`)
  assert.match(focusRule[2], /\b(?:outline|box-shadow)\s*:/, `.${className} focus state should use an outline or box shadow`)
}

const energyCards = homeOpeningViews.filter((tag) => staticClassTokens(tag).includes('energy-card'))
assert.equal(energyCards.length, 4, 'home page should render exactly four energy dashboard cards')
for (const card of energyCards) {
  const ariaLabel = card.match(/\saria-label=["']([^"']*)["']/)?.[1]
  assert.ok(ariaLabel?.trim(), `energy card should expose a non-empty accessibility label: ${card}`)
  assert.match(card, /\srole=["']button["']/, `energy card should use button semantics: ${card}`)
  assert.match(card, /\shover-class=["']energy-card--pressed["']/, `energy card should expose the shared pressed state: ${card}`)
}

const energyCardContracts = [
  { modifier: 'energy-card--test', label: '开始九型人格测试', handler: 'startTest' },
  { modifier: 'energy-card--relation', label: '打开九型关系合盘', handler: 'goRelation' },
  { modifier: 'energy-card--learn', label: '打开老师课程与课件', handler: 'goLearn' },
  { modifier: 'energy-card--profile', label: '打开我的成长档案', handler: 'goProfile' },
]
for (const { modifier, label, handler } of energyCardContracts) {
  const card = energyCards.find((tag) => staticClassTokens(tag).includes(modifier))
  assert.ok(card, `home page should render the ${modifier} energy card`)
  assert.match(card, new RegExp(`\\saria-label=["']${label}["']`), `${modifier} should expose its exact accessible label`)
  assert.match(card, new RegExp(`\\s@click=["']${handler}["']`), `${modifier} should invoke ${handler}`)
  assertKeyboardViewControl(card, modifier, handler)
}

function assertHomeRoute(handler, navigationMethod, url, description) {
  const body = functionBody(handler)
  assert.ok(body !== undefined, `home page should define ${handler}()`)
  assert.match(
    body,
    new RegExp(`uni\\.${navigationMethod}\\s*\\(\\s*\\{\\s*url:\\s*["']${url}["']\\s*\\}\\s*\\)`),
    description,
  )
}

assertHomeRoute('startTest', 'navigateTo', '/pages/test/test', 'home test action should navigate to the test page')
assertHomeRoute('goRelation', 'navigateTo', '/pages/relation/relation', 'home relation action should navigate to the relation page')
assertHomeRoute('goLearn', 'switchTab', '/pages/learn/learn', 'home learn action should switch to the learn tab')
assertHomeRoute('goProfile', 'switchTab', '/pages/profile/profile', 'home profile action should switch to the profile tab')

const homeProfileAction = findHomeView('home-nav__profile')
assert.ok(homeProfileAction, 'home page should render a profile action in the top navigation')
assert.match(homeProfileAction, /\srole=["']button["']/, 'home profile action should use button semantics')
assert.match(homeProfileAction, /\saria-label=["']打开我的成长档案["']/, 'home profile action should describe the growth profile destination')
assert.match(homeProfileAction, /\s@click=["']goProfile["']/, 'home profile action should open the growth profile')
assert.match(homeProfileAction, /\shover-class=["']home-nav__profile--pressed["']/, 'home profile action should expose pressed feedback')
assertKeyboardViewControl(homeProfileAction, 'home profile action', 'goProfile')

const growthCard = findHomeView('growth-card')
assert.ok(growthCard, 'home page should render a teacher and course growth card')
assert.match(growthCard, /\srole=["']button["']/, 'home growth card should use button semantics')
assert.match(growthCard, /\saria-label=["']打开老师课程与成长内容["']/, 'home growth card should describe the teacher and course destination')
assert.match(growthCard, /\s@click=["']goLearn["']/, 'home growth card should open teacher courses and growth content')
assert.match(growthCard, /\shover-class=["']growth-card--pressed["']/, 'home growth card should expose pressed feedback')
assertKeyboardViewControl(growthCard, 'home growth card', 'goLearn')

const homeOpeningButtons = indexPage.match(/<button\b[^>]*>/g) || []
const heroCta = homeOpeningButtons.find((tag) => staticClassTokens(tag).includes('hero__cta'))
assert.ok(heroCta, 'home hero should render its primary CTA as a button')
assert.match(heroCta, /\s@click=["']startTest["']/, 'home hero CTA should start the personality test')

const homeOpeningImages = indexPage.match(/<image\b[^>]*>/g) || []
const heroWheel = homeOpeningImages.find((tag) => staticClassTokens(tag).includes('hero__wheel'))
assert.ok(heroWheel, 'home hero should render the enneagram wheel image')
assert.match(heroWheel, /\sv-if=["']wheelVisible["']/, 'home hero wheel should only render while its image is available')
assert.match(heroWheel, /\s@error=["']hideWheel["']/, 'home hero wheel should hide itself after image errors')
assert.match(heroWheel, /\slazy-load(?:=|\s|>|$)/, 'home hero wheel should lazy-load')
const heroWheelFallback = findHomeView('hero__wheel-fallback')
assert.ok(heroWheelFallback, 'home hero should render a wheel fallback view')
assert.match(heroWheelFallback, /\sv-else(?:\s|>|$)/, 'home hero wheel fallback should be mutually exclusive with the image')

for (const className of ['hero__wheel', 'hero__wheel-fallback']) {
  const declarations = standaloneStyleDeclarations(className)
  assert.ok(declarations, `.${className} should have a standalone CSS rule`)
  assert.match(declarations, /\bwidth:\s*\d+rpx\s*;/, `.${className} should have a fixed width`)
  assert.match(declarations, /\bheight:\s*\d+rpx\s*;/, `.${className} should have a fixed height`)
}

const reducedMotionBlock = bracedBody(/@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{/.exec(indexPage))
assert.ok(reducedMotionBlock, 'home page should respect reduced motion preferences')
assert.match(reducedMotionBlock, /\.hero__visual\s*\{[^}]*(?:animation|transition):\s*none\s*;/, 'reduced motion styles should disable hero visual motion')

for (const className of ['energy-card--pressed', 'growth-card--pressed', 'home-nav__profile--pressed']) {
  const declarations = standaloneStyleDeclarations(className)
  assert.ok(declarations, `.${className} should have a standalone CSS rule`)
  assert.match(declarations, /\b(?:opacity|transform):/, `.${className} should provide visible pressed feedback`)
}

for (const className of ['home-nav__profile', 'energy-card', 'growth-card', 'hero__cta']) {
  assertVisibleFocusStyle(className)
}

for (const inaccessibleColor of ['#778197', '#7b8496', '#6f778b']) {
  assert.doesNotMatch(indexPage, new RegExp(inaccessibleColor, 'i'), `home page should not use low-contrast light-surface color ${inaccessibleColor}`)
}
assert.match(indexPage, /#64748b/i, 'home page should use #64748b for secondary text on light surfaces')

const heroKickerColor = standaloneStyleDeclarations('hero__kicker')?.match(/\bcolor:\s*([^;]+)\s*;/)?.[1].trim()
const heroKickerRgba = heroKickerColor?.match(/^rgba\(\s*255\s*,\s*255\s*,\s*255\s*,\s*([\d.]+)\s*\)$/i)
const heroKickerUsesAccessibleWhite = /^#(?:fff|ffffff)$/i.test(heroKickerColor || '')
  || (heroKickerRgba && Number(heroKickerRgba[1]) >= 0.9)
assert.ok(heroKickerUsesAccessibleWhite, 'home hero kicker should use white with at least 0.9 opacity')

const energyCardMinHeight = standaloneStyleDeclarations('energy-card')?.match(/\bmin-height:\s*(\d+)rpx/)
assert.ok(energyCardMinHeight && Number(energyCardMinHeight[1]) >= 176, 'energy cards should keep a minimum height of 176rpx')

const profileActionStyle = standaloneStyleDeclarations('home-nav__profile')
const profileActionMinWidth = profileActionStyle?.match(/\bmin-width:\s*(\d+)rpx/)
const profileActionMinHeight = profileActionStyle?.match(/\bmin-height:\s*(\d+)rpx/)
assert.ok(profileActionMinWidth && Number(profileActionMinWidth[1]) >= 88, 'home profile action should keep a minimum width of 88rpx')
assert.ok(profileActionMinHeight && Number(profileActionMinHeight[1]) >= 88, 'home profile action should keep a minimum height of 88rpx')

assert.doesNotMatch(indexPage, /openChatPage|goChat|问 AI|AI 对话|打开 AI 对话/, 'home page must not expose AI chat entry copy or handlers')


assert.match(appleMobileStyle, /\.page-stack\s*\{[\s\S]*safe-area-inset-bottom/, 'page-stack should reserve bottom safe area globally')
assert.match(appleMobileStyle, /\.page-stack\s*\{[\s\S]*var\(--window-bottom,\s*0px\)/, 'page-stack should reserve H5 tabbar/window bottom globally')

function collectVueFiles(dir) {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name)
    return statSync(path).isDirectory()
      ? collectVueFiles(path)
      : path.endsWith('.vue')
        ? [path]
        : []
  })
}

for (const file of collectVueFiles('src/pages')) {
  const source = readFileSync(file, 'utf8')
  const buttons = source.match(/<button\b[\s\S]*?>/g) || []
  for (const button of buttons) {
    if (!button.includes(':loading=')) continue
    assert.match(
      button,
      /\s(?::disabled|disabled)(?:=|\s|>)/,
      `${file} has a loading button without disabled state: ${button}`,
    )
  }
}


for (const file of collectVueFiles('src/pages')) {
  const source = readFileSync(file, 'utf8')
  const images = source.match(/<image\b[\s\S]*?>/g) || []
  for (const image of images) {
    if (image.includes('poster-img')) continue
    assert.match(image, /\slazy-load(?:=|\s|>|$)/, `${file} has image without lazy-load: ${image}`)
  }
}



const bookingPage = readFileSync('src/pages/booking/booking.vue', 'utf8')
assert.match(bookingPage, /userErrorMessage/, 'booking page should surface normalized request errors')
assert.match(bookingPage, /title:\s*userErrorMessage\(e,\s*'提交失败，请重试'\)/, 'booking submit should keep a fallback while showing specific API errors')
assert.match(bookingPage, /class=["'][^"']*card[^"']*ios-card[^"']*["']/, 'booking main form card should opt into iOS card styling')
assert.match(bookingPage, /<button\s+class=["'][^"']*btn-primary[^"']*ios-button[^"']*["'][^>]*@click=["']submit["']/, 'booking submit action should opt into iOS button styling')
assert.match(bookingPage, /fieldErrors/, 'booking page should expose inline field validation errors')
assert.match(bookingPage, /v-if=["']fieldErrors\.contactName["']/, 'booking contact name should render an inline validation error')
assert.match(bookingPage, /v-if=["']fieldErrors\.phone["']/, 'booking phone should render an inline validation error')
assert.match(bookingPage, /:aria-invalid=["']!!fieldErrors\.contactName["']/, 'booking contact name input should expose aria-invalid when invalid')
assert.match(bookingPage, /:aria-invalid=["']!!fieldErrors\.phone["']/, 'booking phone input should expose aria-invalid when invalid')

const learnPage = readFileSync('src/pages/learn/learn.vue', 'utf8')

assert.match(learnPage, /normalizeTeachers/, 'learn page should normalize teacher profile data from site config')
assert.match(learnPage, /normalizeCoursewareItems/, 'learn page should normalize courseware and course data from site config')
assert.match(learnPage, /老师资料/, 'learn page should expose a teacher profile section')
assert.match(learnPage, /课件|课程资料/, 'learn page should expose a courseware/materials section')
assert.match(learnPage, /teacher-card/, 'learn page should render teacher cards')
assert.match(learnPage, /courseware-card/, 'learn page should render courseware cards')
assert.match(learnPage, /<image\b[^>]*class=["'][^"']*teacher-card__avatar[^"']*["'][^>]*lazy-load/, 'teacher avatars should lazy-load')
assert.match(learnPage, /<image\b[^>]*class=["'][^"']*courseware-card__cover[^"']*["'][^>]*lazy-load/, 'courseware covers should lazy-load')

assert.match(indexPage, /老师|导师/, 'home page should emphasize teacher guidance')
assert.match(indexPage, /课件|课程/, 'home page should emphasize courseware and courses')
assert.doesNotMatch(indexPage, /AI 对话/, 'home page primary feature cards should avoid AI-heavy copy')

assert.match(learnPage, /loadError/, 'learn page should expose a non-blocking failure state')
assert.match(learnPage, /v-if="loading"/, 'learn page should render loading placeholders instead of a blank area')
assert.match(learnPage, /@click="loadContent"/, 'learn page should provide retry when site config fails')
assert.match(learnPage, /<button\s+class=["']retry["'][^>]*@click=["']loadContent["']/, 'learn retry action should use button semantics')
assert.match(learnPage, /hover-class=["']retry--hover["']/, 'learn retry action should expose a hover/press state')
assert.match(learnPage, /\.retry\s*\{[\s\S]*min-height:\s*88rpx/, 'learn retry action should keep an 88rpx touch target')
assert.match(learnPage, /\.retry--hover\s*\{[\s\S]*(?:opacity|transform)/, 'learn retry hover state should have visible feedback')
assert.match(learnPage, /class=["'][^"']*card[^"']*ios-card[^"']*section[^"']*["']/, 'learn content sections should opt into iOS card styling')
assert.match(learnPage, /<button\s+class=["'][^"']*btn-primary[^"']*ios-button[^"']*["'][^>]*@click=["']goTest["']/, 'learn primary CTA should opt into iOS button styling')

const profilePage = readFileSync('src/pages/profile/profile.vue', 'utf8')
assert.match(profilePage, /profileLoading/, 'profile page should expose a loading state for non-blocking history fetch')
assert.match(profilePage, /v-if="profileLoading"/, 'profile page should render loading placeholder before empty states')
assert.match(profilePage, /loadTicket/, 'profile page should ignore stale concurrent loads')
assert.match(profilePage, /recordsError/, 'profile records should expose a request failure state')
assert.match(profilePage, /bookingsError/, 'profile bookings should expose a request failure state')
assert.match(profilePage, /v-else-if=["']recordsError["']/, 'profile records should render failure state before empty state')
assert.match(profilePage, /v-else-if=["']bookingsError["']/, 'profile bookings should render failure state before empty state')
assert.match(profilePage, /同步失败，重试/, 'profile history failures should show retry copy instead of empty copy')
assert.match(profilePage, /@click=["']loadAll["']/, 'profile history failure state should provide a retry action')
assert.match(profilePage, /\.sync-retry\s*\{[\s\S]*min-height:\s*88rpx/, 'profile history retry action should keep an 88rpx touch target')
assert.doesNotMatch(profilePage, /listTestRecordsApi\(\)\.catch\(\(\)\s*=>\s*\(\{\s*items:\s*\[\]\s*\}\)\)/, 'profile records request failure must not be converted into an empty list')
assert.doesNotMatch(profilePage, /listBookingsApi\(\)\.catch\(\(\)\s*=>\s*\(\{\s*items:\s*\[\]\s*\}\)\)/, 'profile bookings request failure must not be converted into an empty list')

const testPage = readFileSync('src/pages/test/test.vue', 'utf8')

function vueSection(source, tagName) {
  return source.match(new RegExp(`<${tagName}\\b[^>]*>([\\s\\S]*?)<\\/${tagName}>`))?.[1]
}

function stripMarkupAndCssComments(source) {
  return source
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
}

const testTemplate = stripMarkupAndCssComments(vueSection(testPage, 'template') || '')
const testStyle = stripMarkupAndCssComments(vueSection(testPage, 'style') || '')

function openingTagsFor(source, tagName) {
  const tags = []
  const opening = new RegExp(`<${tagName}\\b`, 'g')
  for (const match of source.matchAll(opening)) {
    let quote = null
    for (let index = match.index; index < source.length; index += 1) {
      const character = source[index]
      if ((character === '"' || character === "'") && (!quote || quote === character)) {
        quote = quote ? null : character
      }
      if (character !== '>' || quote) continue
      tags.push(source.slice(match.index, index + 1))
      break
    }
  }
  return tags
}

function tagAttribute(tag, attribute) {
  const escapedAttribute = attribute.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return tag.match(new RegExp(`\\s${escapedAttribute}=(["'])(.*?)\\1`))?.[2]
}

function pageStyleDeclarationBlocks(source, selector) {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return [...source.matchAll(new RegExp(`^[ \\t]*${escapedSelector}\\s*\\{([^}]*)\\}`, 'gm'))]
    .map((match) => match[1])
}

function pageStyleDeclarations(source, selector) {
  return pageStyleDeclarationBlocks(source, selector).at(-1)
}

function sourceBracedBody(source, match) {
  if (!match) return undefined
  const openingBrace = match.index + match[0].lastIndexOf('{')
  let depth = 0
  for (let index = openingBrace; index < source.length; index += 1) {
    if (source[index] === '{') depth += 1
    if (source[index] !== '}') continue
    depth -= 1
    if (depth === 0) return source.slice(openingBrace + 1, index)
  }
  return undefined
}

function hexToRgb(hex) {
  const normalized = hex.replace('#', '')
  const expanded = normalized.length === 3
    ? normalized.split('').map((character) => character + character).join('')
    : normalized
  return [0, 2, 4].map((offset) => Number.parseInt(expanded.slice(offset, offset + 2), 16) / 255)
}

function relativeLuminance(hex) {
  return hexToRgb(hex)
    .map((channel) => channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4)
    .reduce((sum, channel, index) => sum + channel * [0.2126, 0.7152, 0.0722][index], 0)
}

function contrastRatio(foreground, background) {
  const lighter = Math.max(relativeLuminance(foreground), relativeLuminance(background))
  const darker = Math.min(relativeLuminance(foreground), relativeLuminance(background))
  return (lighter + 0.05) / (darker + 0.05)
}

const commentedSourceFixture = `
<template>
  <!-- <button class="quiz__back" @click="back">旧返回按钮</button> -->
  <view class="fixture" />
</template>
<style>
  /* .quiz__opt { min-height: 112rpx; } */
  .fixture { color: #0f172a; }
</style>
`
const uncommentedFixtureTemplate = stripMarkupAndCssComments(vueSection(commentedSourceFixture, 'template') || '')
const uncommentedFixtureStyle = stripMarkupAndCssComments(vueSection(commentedSourceFixture, 'style') || '')
assert.equal(openingTagsFor(uncommentedFixtureTemplate, 'button').length, 0, 'commented template controls must not satisfy UI contracts')
assert.equal(pageStyleDeclarationBlocks(uncommentedFixtureStyle, '.quiz__opt').length, 0, 'commented CSS rules must not satisfy visual contracts')

assert.match(testPage, /answerLocked/, 'test page should guard rapid repeated option taps')
assert.match(testPage, /clearAdvanceTimer/, 'test page should clear pending navigation timers')
assert.match(testPage, /onUnload/, 'test page should cleanup timers on unload')
assert.match(testTemplate, /class=["'][^"']*test-hero[^"']*nx-page-hero/, 'test page should use the blue-purple hero')
assert.match(testPage, /const total = QUESTIONS\.length/, 'test page should expose a stable total question count')

const progressContainer = testTemplate.match(/<view\b(?=[^>]*class=["'][^"']*quiz__progress-meta[^"']*["'])[^>]*>([\s\S]*?)<\/view>/)
assert.ok(progressContainer, 'quiz should render a bounded progress metadata container')
assert.match(progressContainer[0], /:aria-label=["']`第 \$\{step \+ 1\} 题，共 \$\{total\} 题`["']/, 'quiz progress should expose the full accessible question count')
assert.match(progressContainer[1], /<text\s+class=["']quiz__step["']>第 \{\{ step \+ 1 \}\} 题<\/text>/, 'quiz progress should visibly render the current question')
assert.match(progressContainer[1], /<text\s+class=["']quiz__total["']>\/ 共 \{\{ total \}\} 题<\/text>/, 'quiz progress should visibly render the total question count')

const testButtons = openingTagsFor(testTemplate, 'button')
const genderButtons = testButtons.filter((tag) => staticClassTokens(tag).includes('gender__card'))
assert.equal(genderButtons.length, 2, 'test page should render exactly two native gender buttons')
for (const { modifier, label, handler } of [
  { modifier: 'gender__card--m', label: '选择男生', handler: "start('male')" },
  { modifier: 'gender__card--f', label: '选择女生', handler: "start('female')" },
]) {
  const button = genderButtons.find((tag) => staticClassTokens(tag).includes(modifier))
  assert.ok(button, `test page should render the ${modifier} gender button`)
  assert.ok(staticClassTokens(button).includes('nx-focusable'), `${modifier} should use shared focus behavior`)
  assert.equal(tagAttribute(button, 'aria-label'), label, `${modifier} should expose its exact accessible label`)
  assert.equal(tagAttribute(button, '@click'), handler, `${modifier} should invoke its exact start handler`)
}

const quizOption = testButtons.find((tag) => tagAttribute(tag, 'v-for') === '(opt, k) in q.options')
assert.ok(quizOption, 'test page should render quiz options as native buttons')
assert.equal(tagAttribute(quizOption, 'class'), 'quiz__opt nx-focusable', 'quiz options should use the exact focusable panel classes')
assert.equal(tagAttribute(quizOption, ':class'), '{ on: answers[step] === opt, disabled: answerLocked }', 'quiz options should preserve selected and locked classes')
assert.equal(tagAttribute(quizOption, ':disabled'), 'answerLocked', 'quiz options should preserve the rapid-tap lock')
assert.equal(tagAttribute(quizOption, ':aria-label'), "'选择答案 ' + letter(k) + '：' + opt.t", 'quiz options should describe each answer')
assert.equal(tagAttribute(quizOption, '@click'), 'choose(opt)', 'quiz options should preserve the answer handler')

const quizBackButton = testButtons.find((tag) => staticClassTokens(tag).includes('quiz__back'))
assert.ok(quizBackButton, 'quiz previous action should be a native button element')
assert.equal(tagAttribute(quizBackButton, '@click'), 'back', 'quiz previous button should invoke the back handler on itself')
const quizBackTouchStyle = pageStyleDeclarationBlocks(testStyle, '.quiz__back')
  .find((declarations) => /min-height:/.test(declarations))
assert.match(quizBackTouchStyle, /min-height:\s*88rpx\s*;/, 'quiz back action should keep an 88rpx touch target')

const testOpeningViews = openingTagsFor(testTemplate, 'view')
const quizShellTag = testOpeningViews.find((tag) => staticClassTokens(tag).includes('quiz-shell'))
assert.ok(quizShellTag, 'test quiz should use its dedicated light surface')
assert.ok(!staticClassTokens(quizShellTag).includes('card'), 'quiz shell should not use the generic card class')

const heroStyle = pageStyleDeclarations(testStyle, '.test-hero')
assert.ok(heroStyle, 'test hero should define a standalone style rule')
assert.match(heroStyle, /background:\s*linear-gradient\(135deg,\s*#1d4ed8\s+0%,[\s\S]*#6d28d9\s+100%\)/i, 'test hero should keep the exact blue-to-purple gradient endpoints')
for (const selector of ['.test-hero__eyebrow', '.test-hero__title', '.test-hero__desc']) {
  const declarations = pageStyleDeclarations(testStyle, selector)
  assert.ok(declarations, `${selector} should define a standalone style rule`)
  assert.match(declarations, /color:\s*(?:#fff(?:fff)?|rgba\(\s*255\s*,\s*255\s*,\s*255\s*,\s*\.9\d*\s*\))/i, `${selector} should keep accessible white hero text`)
}

const genderRowStyle = pageStyleDeclarations(testStyle, '.gender__row')
assert.match(genderRowStyle, /display:\s*flex\s*;/, 'gender choices should stay in a two-column flex row')
const genderCardStyle = pageStyleDeclarations(testStyle, '.gender__card')
assert.match(genderCardStyle, /flex:\s*1\s*;/, 'gender cards should share the row as two equal columns')
const genderCardMinHeight = genderCardStyle?.match(/min-height:\s*(\d+)rpx\s*;/)
assert.ok(genderCardMinHeight && Number(genderCardMinHeight[1]) >= 230, 'gender cards should keep at least 230rpx height')
assert.match(pageStyleDeclarations(testStyle, '.gender__card--m'), /background:\s*linear-gradient\(145deg,\s*#155e75,\s*#1d4ed8\)\s*;/i, 'male gender card should keep the teal-to-blue gradient')
assert.match(pageStyleDeclarations(testStyle, '.gender__card--f'), /background:\s*linear-gradient\(145deg,\s*#7e22ce,\s*#be185d\)\s*;/i, 'female gender card should keep the purple-to-pink gradient')

const quizShellStyle = pageStyleDeclarations(testStyle, '.quiz-shell')
assert.match(quizShellStyle, /background:\s*rgba\(\s*255\s*,\s*255\s*,\s*255\s*,\s*\.9\d*\s*\)\s*;/i, 'quiz shell should keep a light surface')
const quizOptionStyle = pageStyleDeclarationBlocks(testStyle, '.quiz__opt')
  .find((declarations) => /min-height:\s*112rpx\s*;/.test(declarations))
assert.match(quizOptionStyle, /min-height:\s*112rpx\s*;/, 'quiz options should keep a 112rpx touch surface')
assert.match(quizOptionStyle, /border-radius:\s*24rpx\s*;/, 'quiz options should keep a 24rpx radius')
const selectedOptionStyle = pageStyleDeclarations(testStyle, '.quiz__opt.on')
assert.match(selectedOptionStyle, /border:\s*4rpx\s+solid\s+#4f46e5\s*;/i, 'selected answers should keep the 4rpx blue-purple border')
assert.match(selectedOptionStyle, /box-shadow:[\s\S]*\binset\b/i, 'selected answers should keep a non-color-only inset ring')

const genderTipColor = pageStyleDeclarations(testStyle, '.gender__tip')?.match(/color:\s*(#[\da-f]{6})\s*;/i)?.[1]
const testPageBackground = pageStyleDeclarations(testStyle, '.test')?.match(/background:[\s\S]*,\s*(#[\da-f]{6})\s*;/i)?.[1]
assert.ok(genderTipColor && testPageBackground, 'gender helper text should expose parseable foreground and page background colors')
assert.ok(
  contrastRatio(genderTipColor, testPageBackground) >= 4.5,
  `gender helper text contrast should be at least 4.5:1, got ${contrastRatio(genderTipColor, testPageBackground).toFixed(2)}:1`,
)
assert.match(pageStyleDeclarations(testStyle, '.quiz__eyebrow'), /color:\s*#64748b\s*;/i, '.quiz__eyebrow should keep accessible secondary text')
assert.match(pageStyleDeclarations(testStyle, '.quiz__t'), /color:\s*#334155\s*;/i, 'quiz answer text should stay darker than secondary text')

for (const selector of ['.gender__d', '.gender__go']) {
  const fontSize = pageStyleDeclarations(testStyle, selector)?.match(/font-size:\s*(\d+)rpx\s*;/)
  assert.ok(fontSize && Number(fontSize[1]) >= 24, `${selector} should keep at least 24rpx readable text`)
}

const compactMedia = sourceBracedBody(testStyle, /@media\s*\(max-width:\s*360px\)\s*\{/.exec(testStyle))
assert.ok(compactMedia, 'test page should define the 360px compact breakpoint')
const compactRules = [...compactMedia.matchAll(/([^{}]+)\{([^{}]*)\}/g)]
assert.equal(compactRules.length, 1, 'compact breakpoint should contain only the question typography rule')
assert.equal(compactRules[0][1].trim(), '.quiz__q', 'compact breakpoint should only target the question text')
assert.match(compactRules[0][2].trim(), /^font-size:\s*36rpx\s*;$/, 'compact screens should only reduce the question font size')
assert.doesNotMatch(compactMedia, /\b(?:min-)?height\s*:/, 'compact breakpoint must not reduce any touch height')
assert.doesNotMatch(compactMedia, /\.(?:quiz__back|quiz__opt)\b/, 'compact breakpoint must not override quiz touch controls')

assert.match(learnPage, /getStoredSiteConfig/, 'learn page should render stored site config before network refresh')
assert.match(learnPage, /refreshSiteConfig/, 'learn page should refresh site config in the background')
assert.match(learnPage, /silent/, 'learn background refresh should avoid replacing cached content with a blocking state')

assert.match(profilePage, /wechatLoginReady/, 'profile page should expose a WeChat login integration slot')
assert.match(profilePage, /open-type="chooseAvatar"/, 'profile page should keep WeChat avatar slot')
assert.match(profilePage, /type="nickname"/, 'profile page should keep WeChat nickname slot')
assert.doesNotMatch(profilePage, /open-type="getPhoneNumber"/, '未接通后端前，手机号授权入口不能对用户露出')
assert.doesNotMatch(profilePage, /@getphonenumber="onGetPhoneNumber"/, '未接通后端前，不应绑定可见手机号授权占位事件')
assert.match(profilePage, /#ifdef H5[\s\S]*请在微信小程序内登录[\s\S]*#endif/, 'H5 profile login entry should be a disabled miniapp guidance instead of a failing WeChat login CTA')
assert.doesNotMatch(profilePage, /后端暂未开通|前端占位|占位/, '用户侧文案不能暴露手机号授权后端占位状态')
assert.doesNotMatch(profilePage, /openChatPage|goChat|clearChatMessages|问 AI|AI 对话/, 'profile page must not expose or reset removed AI chat state')


const resultPage = readFileSync('src/pages/result/result.vue', 'utf8')
const resultTemplateRaw = resultPage.match(/<template>([\s\S]*)<\/template>\s*<style/)?.[1] || ''
const resultTemplate = stripMarkupAndCssComments(resultTemplateRaw)
const resultStyle = stripMarkupAndCssComments(vueSection(resultPage, 'style') || '')
const resultViews = openingTagsFor(resultTemplate, 'view')

assert.match(resultPage, /import\s*\{\s*reportDisplayState\s*\}\s*from\s*['"]\.\.\/\.\.\/utils\/reportDisplayState['"]/, 'result page should use the pure report display-state helper')
assert.match(resultPage, /const reportPriceCents = ref\(null\)/, 'result price should start unknown instead of assuming a charge')
assert.match(resultPage, /const reportStatusLoading = ref\(false\)/, 'result page should track report status loading separately')
assert.match(resultPage, /const reportStatusError = ref\(['"]['"]\)/, 'result page should expose report status errors')
assert.match(resultPage, /const reportState = computed\(\(\) => reportDisplayState\(\{[\s\S]*recordId:[\s\S]*loading:\s*reportStatusLoading\.value,[\s\S]*error:\s*reportStatusError\.value,[\s\S]*unlocked:\s*reportUnlocked\.value,[\s\S]*priceCents:\s*reportPriceCents\.value,[\s\S]*\}\)\)/, 'result page should derive its report UI from the pure five-state helper')
assert.doesNotMatch(resultPage, /ref\(990\)/, 'result page must not hard-code a default report price')

for (const className of ['result-hero', 'drive-grid', 'center-panel', 'direction-grid', 'report-panel']) {
  assert.ok(resultViews.some((tag) => staticClassTokens(tag).includes(className)), `result page should render ${className}`)
}
assert.match(resultTemplate, /class=["'][^"']*result-hero[^"']*nx-page-hero[^"']*["']/, 'result hero should use the shared hero surface')
assert.match(resultTemplate, /class=["']result-hero[^"']*["']\s+:class=["']`result-hero--\$\{info\.color\}`["']/, 'result hero should use the personality color modifier')
assert.match(resultPage, /const avatarFailed = ref\(false\)/, 'result page should track avatar load failure')
const resultAvatar = openingTagsFor(resultTemplate, 'image').find((tag) => staticClassTokens(tag).includes('result-hero__avatar'))
assert.ok(resultAvatar, 'result hero should render a fixed avatar image')
assert.equal(tagAttribute(resultAvatar, 'v-if'), '!avatarFailed', 'result avatar should be replaced after an image error')
assert.equal(tagAttribute(resultAvatar, '@error'), 'avatarFailed = true', 'result avatar should record image errors')
assert.match(resultAvatar, /\slazy-load(?:=|\s|>|$)/, 'result avatar should lazy-load')
const resultAvatarFallback = resultViews.find((tag) => staticClassTokens(tag).includes('result-hero__avatar-fallback'))
assert.ok(resultAvatarFallback && /\sv-else(?:\s|>|$)/.test(resultAvatarFallback), 'result hero should render a mutually exclusive avatar fallback')
for (const selector of ['.result-hero__avatar', '.result-hero__avatar-fallback']) {
  const declarations = pageStyleDeclarations(resultStyle, selector)
  assert.match(declarations, /width:\s*184rpx\s*;/, `${selector} should reserve 184rpx width`)
  assert.match(declarations, /height:\s*184rpx\s*;/, `${selector} should reserve 184rpx height`)
}

const reportPanel = resultTemplate.match(/<view\s+class=["']report-panel["']>([\s\S]*?)<view\s+class=["']result-actions["']>/)?.[1]
assert.ok(reportPanel, 'result page should expose one bounded report panel')
for (const state of ['needs-save', 'status-loading', 'status-error', 'ready']) {
  assert.match(reportPanel, new RegExp(`reportState\\.key === '${state}'`), `report panel should render ${state}`)
}
assert.match(reportPanel, /<template\s+v-else>/, 'report panel final branch should render the unlocked state')
assert.equal((reportPanel.match(/@click=["']saveRecord["']/g) || []).length, 1, 'save should exist exactly once inside needs-save')
assert.equal((reportPanel.match(/@click=["']unlockReport["']/g) || []).length, 1, 'unlock should exist exactly once inside ready')
assert.equal((resultTemplate.match(/@click=["']saveRecord["']/g) || []).length, 1, 'result page should not duplicate its save CTA outside the report panel')
assert.equal((resultTemplate.match(/@click=["']unlockReport["']/g) || []).length, 1, 'result page should not duplicate its unlock CTA outside the report panel')
assert.match(reportPanel, /aria-live=["']polite["'][^>]*>\s*查询报告状态/, 'report status loading should be announced politely')
assert.match(reportPanel, /report__retry[^>]*@click=["']refreshReportStatus["']/, 'report status failure should allow retrying status fetch')
assert.match(reportPanel, /report__content-retry[^>]*@click=["']loadReportContent["']/, 'unlocked content failure should allow retrying content fetch')

const resultH5Blocks = [...resultTemplateRaw.matchAll(/<!-- #ifdef H5 -->([\s\S]*?)<!-- #endif -->/g)].map((match) => match[1])
const h5SaveBlock = resultH5Blocks.find((block) => block.includes('请在微信小程序内登录后保存'))
assert.ok(h5SaveBlock, 'H5 needs-save should explain that saving requires the miniapp')
assert.match(h5SaveBlock, /<button\b[^>]*\sdisabled(?:\s|>)/, 'H5 save guidance should be disabled')
assert.doesNotMatch(h5SaveBlock, /@click=/, 'H5 save guidance must not bind a save handler')
const h5PaymentBlock = resultH5Blocks.find((block) => block.includes('请在微信小程序内完成存档与支付'))
assert.ok(h5PaymentBlock, 'H5 ready state should explain that payment requires the miniapp')
assert.match(h5PaymentBlock, /<button\b[^>]*\sdisabled(?:\s|>)/, 'H5 payment guidance should be disabled')
assert.doesNotMatch(h5PaymentBlock, /@click=/, 'H5 payment guidance must not bind a payment handler')
const h5PosterBlock = resultH5Blocks.find((block) => block.includes('小程序内生成海报'))
assert.ok(h5PosterBlock && /\sdisabled(?:\s|>)/.test(h5PosterBlock), 'H5 poster action should remain disabled guidance')
assert.doesNotMatch(resultH5Blocks.join('\n'), /open-type=["']share["']/, 'H5 should not expose miniapp sharing')

const resultMpBlocks = [...resultTemplateRaw.matchAll(/<!-- #ifdef MP-WEIXIN -->([\s\S]*?)<!-- #endif -->/g)].map((match) => match[1])
assert.ok(resultMpBlocks.some((block) => /open-type=["']share["']/.test(block)), 'WeChat should preserve friend sharing')
assert.ok(resultMpBlocks.some((block) => /@click=["']saveRecord["']/.test(block)), 'WeChat should preserve saving')
assert.ok(resultMpBlocks.some((block) => /@click=["']unlockReport["']/.test(block)), 'WeChat should preserve report payment')

const refreshStatusBody = sourceBracedBody(resultPage, /async function\s+refreshReportStatus\s*\(\s*\)\s*\{/.exec(resultPage))
assert.match(refreshStatusBody, /reportStatusLoading\.value\s*=\s*true/, 'report status refresh should enter loading state')
assert.match(refreshStatusBody, /reportStatusError\.value\s*=\s*['"]['"]/, 'report status refresh should clear its prior error')
assert.match(refreshStatusBody, /reportUnlocked\.value\s*=\s*!!st\.unlocked/, 'report status refresh should apply unlocked before validating price')
assert.match(refreshStatusBody, /Number\.isFinite\(st\.priceCents\)[\s\S]*st\.priceCents\s*>\s*0/, 'locked report should accept only a finite positive price')
assert.match(refreshStatusBody, /finally\s*\{[\s\S]*reportStatusLoading\.value\s*=\s*false/, 'report status refresh should always stop loading')
assert.match(resultPage, /const reportPriceYuan = computed\(\(\) => \{[\s\S]*Number\.isFinite\(reportPriceCents\.value\)[\s\S]*reportPriceCents\.value\s*>\s*0[\s\S]*return ''/, 'report price display should stay blank until a valid positive price is known')

const saveRecordBody = sourceBracedBody(resultPage, /async function\s+saveRecord\s*\(\s*\)\s*\{/.exec(resultPage))
assert.match(saveRecordBody, /if\s*\(\s*!rec\s*\|\|\s*!rec\.id\s*\)\s*\{?\s*throw new Error\(['"]存档失败，请重试['"]\)/, 'save should reject an API response without a record id')
const invalidRecordGuardIndex = saveRecordBody.indexOf('if (!rec || !rec.id)')
const assignRecordIdIndex = saveRecordBody.indexOf('recordId.value = rec.id')
const markSavedIndex = saveRecordBody.indexOf('saved.value = true')
const successToastIndex = saveRecordBody.indexOf("uni.showToast({ title: '已存入我的档案', icon: 'success' })")
const refreshAfterSaveIndex = saveRecordBody.indexOf('await refreshReportStatus()')
assert.ok(invalidRecordGuardIndex >= 0 && invalidRecordGuardIndex < assignRecordIdIndex, 'save should validate the record id before storing it')
assert.ok(assignRecordIdIndex < markSavedIndex, 'save should store the valid record id before marking the result saved')
assert.ok(markSavedIndex < successToastIndex, 'save should mark success before showing its success toast')
assert.ok(successToastIndex < refreshAfterSaveIndex, 'save should acknowledge the valid archive before awaiting report status')
assert.match(saveRecordBody, /catch\s*\(e\)\s*\{[\s\S]*userErrorMessage\(e,\s*['"]存档失败，请重试['"]\)/, 'save failures should use the normalized fallback message')
assert.match(saveRecordBody, /finally\s*\{[\s\S]*saving\.value\s*=\s*false/, 'save should always restore its loading guard')

const loadReportBody = sourceBracedBody(resultPage, /async function\s+loadReportContent\s*\(\s*\)\s*\{/.exec(resultPage))
assert.match(loadReportBody, /if\s*\(reportLoading\.value\s*\|\|\s*reportContent\.value\)\s*return/, 'report content loading should retain its duplicate-request guard')
const unlockReportBody = sourceBracedBody(resultPage, /async function\s+unlockReport\s*\(\s*\)\s*\{/.exec(resultPage))
assert.match(unlockReportBody, /reportUnlocked\.value\s*=\s*true[\s\S]*loadReportContent\(\)/, 'successful unlock should still load report content')

const reportStyle = pageStyleDeclarations(resultStyle, '.report-panel')
assert.match(reportStyle, /background:\s*linear-gradient\(135deg,\s*#111827\s+0%,\s*#312e81\s+100%\)\s*;/i, 'report panel should keep the exact dark indigo gradient')
assert.match(reportStyle, /border-radius:\s*34rpx\s*;/, 'report panel should use the planned 34rpx radius')
assert.match(reportStyle, /padding:\s*34rpx\s*;/, 'report panel should use the planned 34rpx padding')
for (const selector of ['.report__cta', '.report__secondary', '.result-actions button', '.restart-button']) {
  const declarations = pageStyleDeclarations(resultStyle, selector)
  assert.match(declarations, /min-height:\s*88rpx\s*;/, `${selector} should keep an 88rpx touch target`)
}
for (const selector of ['.report__intro', '.report__status', '.report__error', '.report__content', '.disclaimer']) {
  const fontSize = pageStyleDeclarations(resultStyle, selector)?.match(/font-size:\s*(\d+)rpx\s*;/)
  assert.ok(fontSize && Number(fontSize[1]) >= 24, `${selector} should keep at least 24rpx readable text`)
}
assert.match(pageStyleDeclarations(resultStyle, '.drive-grid'), /grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\)\s*;/, 'result drives should stay in equal columns')
assert.match(pageStyleDeclarations(resultStyle, '.direction-grid'), /grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\)\s*;/, 'result directions should stay in equal columns')
assert.match(resultTemplate, /aria-label=["']关闭海报["']/, 'poster close action should expose an accessible label')
assert.match(resultPage, /userErrorMessage/, 'result page should surface normalized request errors')
assert.match(resultPage, /normalizeLastResult/, 'result page should validate cached result schema before rendering')
assert.match(resultPage, /测试结果已失效/, 'result page should give feedback when cached result schema is invalid')

const relationPage = readFileSync('src/pages/relation/relation.vue', 'utf8')
const relationTemplate = stripMarkupAndCssComments(relationPage.match(/<template>([\s\S]*)<\/template>\s*<style/)?.[1] || '')
const relationStyle = stripMarkupAndCssComments(vueSection(relationPage, 'style') || '')

function elementBlocksByStaticClass(source, tagName, className) {
  const escapedTagName = tagName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const tags = [...source.matchAll(new RegExp(`<\\/?${escapedTagName}\\b[^>]*>`, 'g'))]
  const blocks = []
  for (let startIndex = 0; startIndex < tags.length; startIndex += 1) {
    const openingTag = tags[startIndex][0]
    if (openingTag.startsWith('</') || openingTag.endsWith('/>')) continue
    if (!staticClassTokens(openingTag).includes(className)) continue
    let depth = 1
    for (let endIndex = startIndex + 1; endIndex < tags.length; endIndex += 1) {
      const tag = tags[endIndex][0]
      if (tag.startsWith('</')) depth -= 1
      else if (!tag.endsWith('/>')) depth += 1
      if (depth !== 0) continue
      blocks.push(source.slice(tags[startIndex].index, tags[endIndex].index + tag.length))
      break
    }
  }
  return blocks
}

const enneagramGameSource = readFileSync('src/data/enneagramGame.js', 'utf8')
const typesInfoSource = enneagramGameSource.match(/export const TYPES_INFO\s*=\s*\{([\s\S]*?)\n\}\n\nexport const CENTERS/)?.[1] || ''
const typeIds = [...typesInfoSource.matchAll(/^\s{2}([1-9]):\s*\{/gm)].map((match) => Number(match[1]))
assert.deepEqual(typeIds, [1, 2, 3, 4, 5, 6, 7, 8, 9], 'enneagram type data should expose every type id from 1 through 9')
assert.match(
  relationPage,
  /^const[ \t]+allTypes[ \t]*=[ \t]*Object\.keys\(TYPES_INFO\)\.map\([ \t]*\(id\)[ \t]*=>[ \t]*\(\{[ \t]*id:[ \t]*Number\(id\),[ \t]*\.\.\.TYPES_INFO\[id\][ \t]*\}\)[ \t]*\)[ \t]*;?[ \t]*$/m,
  'relation allTypes should map every TYPES_INFO entry without trailing slice or filter chains',
)
assert.match(relationPage, /<view\s+class=["'][^"']*page-stack[^"']*ios-page[^"']*ios-safe-bottom[^"']*["']/, 'relation root should use shared page-stack/iOS safe-area classes')
assert.match(relationPage, /<button\s+class=["'][^"']*btn-primary[^"']*ios-button[^"']*["'][^>]*@click=["']analyze["']/, 'relation primary action should opt into iOS button styling')
assert.doesNotMatch(relationPage, /padding-bottom:\s*60rpx/, 'relation page should not hard-code bottom padding outside shared safe-area helpers')
const relationGridGap = relationPage.match(/\.grid\s*\{[\s\S]*?gap:\s*(\d+)rpx/)
assert.ok(relationGridGap && Number(relationGridGap[1]) >= 16, 'relation type grid gap should be at least 16rpx')
assert.match(relationPage, /isValidTypeId/, 'relation page should validate incoming and selected type ids')
assert.match(relationPage, /stage\.value\s*=\s*'redirecting'/, 'relation invalid query should enter a redirecting state instead of leaving the pick UI interactive')
assert.match(relationPage, /v-else-if="stage === 'result'"/, 'relation result view should be explicit so redirecting can show a safe placeholder')
assert.match(relationPage, /型号参数无效/, 'relation page should explain invalid query type before navigation')
assert.match(relationPage, /\/pages\/test\/test/, 'relation page should return to the test page for invalid query type')
assert.match(relationTemplate, /class=["'][^"']*relation-hero[^"']*nx-page-hero[^"']*["']/, 'relation pick stage should use a themed hero')

const relationViews = openingTagsFor(relationTemplate, 'view')
const typePickers = relationViews.filter((tag) => staticClassTokens(tag).includes('type-picker'))
assert.equal(typePickers.length, 2, 'relation should render exactly two type pickers')
for (const picker of typePickers) {
  assert.ok(staticClassTokens(picker).includes('nx-panel'), 'each relation type picker should use the shared panel surface')
}

const relationButtons = openingTagsFor(relationTemplate, 'button')
assert.equal(relationButtons.filter((tag) => tagAttribute(tag, '@click') === 'analyze').length, 1, 'relation pick stage should keep exactly one primary analyze action')
const typeChips = relationButtons.filter((tag) => tagAttribute(tag, 'v-for') === 't in allTypes')
assert.equal(typeChips.length, 2, 'relation should render one native type-chip loop for each person')
for (const { keyPrefix, ariaLabel, ariaPressed, handler } of [
  { keyPrefix: "'m' + t.id", ariaLabel: '`选择我的型号 ${t.id} ${t.name}`', ariaPressed: 'myType === t.id', handler: 'pickMy(t.id)' },
  { keyPrefix: "'t' + t.id", ariaLabel: '`选择 TA 的型号 ${t.id} ${t.name}`', ariaPressed: 'taType === t.id', handler: 'pickTa(t.id)' },
]) {
  const chip = typeChips.find((tag) => tagAttribute(tag, ':key') === keyPrefix)
  assert.ok(chip, `relation should render the ${keyPrefix} type-chip loop`)
  assert.equal(tagAttribute(chip, 'class'), 'type-chip nx-focusable', 'relation type chips should use the exact shared focusable classes')
  assert.equal(tagAttribute(chip, ':aria-label'), ariaLabel, 'relation type chip should keep its accessible label on the native button')
  assert.equal(tagAttribute(chip, ':aria-pressed'), ariaPressed, 'relation type chip should expose its selected state on the native button')
  assert.equal(tagAttribute(chip, 'hover-class'), 'type-chip--pressed', 'relation type chip should expose pressed feedback on the native button')
  assert.equal(tagAttribute(chip, '@click'), handler, 'relation type chip should keep its selection handler on the native button')
  assert.equal(tagAttribute(chip, 'role'), 'button', 'relation type chip should expose H5 button semantics')
  assert.equal(tagAttribute(chip, 'aria-role'), 'button', 'relation type chip should expose miniapp button semantics')
  assert.equal(tagAttribute(chip, 'tabindex'), '0', 'relation type chip should participate in H5 keyboard focus order')
  assert.equal(tagAttribute(chip, '@keydown.enter'), handler, 'relation type chip should activate with Enter')
  assert.equal(tagAttribute(chip, '@keydown.space.prevent'), handler, 'relation type chip should activate with Space without scrolling')
}

for (const { handler, description } of [
  { handler: 'analyze', description: 'relation analyze action' },
  { handler: 'reset', description: 'relation reset action' },
]) {
  const button = relationButtons.find((tag) => tagAttribute(tag, '@click') === handler)
  assert.ok(button, `${description} should be a native button`)
  assert.equal(tagAttribute(button, 'role'), 'button', `${description} should expose H5 button semantics`)
  assert.equal(tagAttribute(button, 'aria-role'), 'button', `${description} should expose miniapp button semantics`)
  assert.equal(tagAttribute(button, 'tabindex'), '0', `${description} should participate in H5 keyboard focus order`)
  assert.equal(tagAttribute(button, '@keydown.enter'), handler, `${description} should activate with Enter`)
  assert.equal(tagAttribute(button, '@keydown.space.prevent'), handler, `${description} should activate with Space without scrolling`)
}

const chipBodies = [...relationTemplate.matchAll(/<button\b(?=[^>]*class=["']type-chip nx-focusable["'])[^>]*>([\s\S]*?)<\/button>/g)]
assert.equal(chipBodies.length, 2, 'relation should keep selected text inside both type-chip templates')
for (const [, body] of chipBodies) {
  assert.match(body, /<text\s+class=["']type-chip__number["']>\{\{ t\.id \}\}<\/text>/, 'type chip should visibly render its number')
  assert.match(body, /<text\s+class=["']type-chip__name["']>\{\{ t\.name \}\}<\/text>/, 'type chip should visibly render its abbreviated name')
  assert.match(body, /<text\s+v-if=["'][^"']+ === t\.id["']\s+class=["']type-chip__selected["']>已选<\/text>/, 'selected chip should include a visible text marker')
  assert.match(body, /<text\s+v-else\s+class=["']type-chip__selected type-chip__selected--placeholder["']\s+aria-hidden=["']true["']>/, 'unselected chip should reserve the selected-marker layout slot')
}

const typeChipStyle = pageStyleDeclarations(relationStyle, '.type-chip')
const typeChipHeight = typeChipStyle?.match(/height:\s*(\d+)rpx\s*;/)
const typeChipMinHeight = typeChipStyle?.match(/min-height:\s*(\d+)rpx\s*;/)
assert.ok(typeChipHeight && typeChipMinHeight, 'relation type chips should define stable height and minimum height')
assert.equal(typeChipHeight[1], typeChipMinHeight[1], 'selected and unselected relation chips should share one stable height')
assert.ok(Number(typeChipHeight[1]) >= 88, 'relation type chips should keep at least an 88rpx touch target')
assert.match(typeChipStyle, /border-radius:\s*24rpx\s*;/, 'relation type chips should keep the planned 24rpx radius')
assert.match(pageStyleDeclarations(relationStyle, '.type-chip__selected--placeholder'), /visibility:\s*hidden\s*;/, 'unselected relation chips should reserve marker height without showing placeholder copy')
const selectedChipStyle = pageStyleDeclarations(relationStyle, '.type-chip.on')
assert.match(selectedChipStyle, /border:\s*4rpx\s+solid\s+#9333ea\s*;/i, 'selected relation type should keep the planned purple border')
assert.match(selectedChipStyle, /box-shadow:[^;]*\binset\b/i, 'selected relation type should include a non-color-only inset emphasis')
assert.match(pageStyleDeclarations(relationStyle, '.type-chip--pressed'), /(?:opacity|transform)\s*:/, 'relation chip press state should have visible feedback')

const relationHeroStyle = pageStyleDeclarations(relationStyle, '.relation-hero')
assert.match(relationHeroStyle, /background:\s*linear-gradient\(135deg,\s*#6d28d9\s+0%,\s*#db2777\s+100%\)\s*;/i, 'relation hero should keep the exact purple-to-pink gradient')
for (const selector of ['.relation-hero__eyebrow', '.relation-hero__title', '.relation-hero__desc']) {
  assert.match(pageStyleDeclarations(relationStyle, selector), /color:\s*(?:#fff(?:fff)?|rgba\(\s*255\s*,\s*255\s*,\s*255\s*,\s*\.9\d*\s*\))\s*;/i, `${selector} should keep accessible white hero text`)
}

assert.match(relationPage, /const myAvatarFailed = ref\(false\)/, 'relation should track my avatar failure')
assert.match(relationPage, /const taAvatarFailed = ref\(false\)/, 'relation should track TA avatar failure')
assert.match(sourceBracedBody(relationPage, /function\s+analyze\s*\(\s*\)\s*\{/.exec(relationPage)), /myAvatarFailed\.value\s*=\s*false[\s\S]*taAvatarFailed\.value\s*=\s*false/, 'relation analyze should reset both avatar failure states')
assert.match(sourceBracedBody(relationPage, /function\s+reset\s*\(\s*\)\s*\{/.exec(relationPage)), /myAvatarFailed\.value\s*=\s*false[\s\S]*taAvatarFailed\.value\s*=\s*false/, 'relation reset should clear both avatar failure states')

const relationImages = openingTagsFor(relationTemplate, 'image')
assert.equal(relationImages.length, 2, 'relation result should render exactly two avatar image templates')
for (const { condition, errorHandler } of [
  { condition: '!myAvatarFailed', errorHandler: 'onMyAvatarError' },
  { condition: '!taAvatarFailed', errorHandler: 'onTaAvatarError' },
]) {
  const avatar = relationImages.find((tag) => tagAttribute(tag, 'v-if') === condition)
  assert.ok(avatar, `relation should render the ${condition} avatar while available`)
  assert.ok(staticClassTokens(avatar).includes('pair__avatar'), 'relation avatars should use the fixed avatar class')
  assert.equal(tagAttribute(avatar, '@error'), errorHandler, 'relation avatar should set its failure flag on image error')
  assert.match(avatar, /\slazy-load(?:=|\s|>|$)/, 'relation avatars should lazy-load')
}

const avatarFallbacks = relationViews.filter((tag) => staticClassTokens(tag).includes('pair__avatar-fallback'))
assert.equal(avatarFallbacks.length, 2, 'relation should render one fallback for each avatar')
for (const fallback of avatarFallbacks) {
  assert.ok(/\sv-else(?:\s|>|$)/.test(fallback), 'relation avatar fallback should be mutually exclusive with its image')
}
const avatarFallbackBlocks = elementBlocksByStaticClass(relationTemplate, 'view', 'pair__avatar-fallback')
assert.equal(avatarFallbackBlocks.length, 2, 'relation should expose two bounded avatar fallback elements')
assert.match(avatarFallbackBlocks[0], /^<view\b[^>]*>\{\{ myInfo\.id \}\}<\/view>$/, 'my avatar fallback should display my type number within its own element')
assert.match(avatarFallbackBlocks[1], /^<view\b[^>]*>\{\{ taInfo\.id \}\}<\/view>$/, 'TA avatar fallback should display TA type number within its own element')
for (const selector of ['.pair__avatar', '.pair__avatar-fallback']) {
  const declarations = pageStyleDeclarations(relationStyle, selector)
  assert.match(declarations, /width:\s*112rpx\s*;/, `${selector} should keep the planned fixed width`)
  assert.match(declarations, /height:\s*112rpx\s*;/, `${selector} should keep the planned fixed height`)
}

const pairHero = relationViews.find((tag) => staticClassTokens(tag).includes('pair'))
assert.ok(pairHero && staticClassTokens(pairHero).includes('nx-page-hero'), 'relation result pair should use the shared hero container')
assert.ok(relationViews.some((tag) => staticClassTokens(tag).includes('pair-connection')), 'relation result should render the connection visual')
const pairConnectionBlocks = elementBlocksByStaticClass(relationTemplate, 'view', 'pair-connection')
assert.equal(pairConnectionBlocks.length, 1, 'relation result should render exactly one bounded connection visual')
assert.match(pairConnectionBlocks[0], /<text\s+class=["']pair-connection__score["']>\{\{ analysis\.score \}\}<\/text>/, 'connection visual should bind the analysis score inside its own container')
assert.match(pairConnectionBlocks[0], /<text\s+class=["']pair-connection__label["']>契合指数<\/text>/, 'connection visual should label the score inside its own container')

const insightContracts = [
  { modifier: 'insight--bond', binding: 'analysis.bond' },
  { modifier: 'insight--friction', binding: 'analysis.friction' },
  { modifier: 'insight--tip', binding: 'analysis.tip' },
]
for (const { modifier, binding } of insightContracts) {
  assert.ok(relationViews.some((tag) => staticClassTokens(tag).includes(modifier)), `relation result should include ${modifier}`)
  const blocks = elementBlocksByStaticClass(relationTemplate, 'view', modifier)
  assert.equal(blocks.length, 1, `relation result should render exactly one bounded ${modifier} panel`)
  assert.match(blocks[0], new RegExp(`<text\\s+class=["']insight__text["']>\\{\\{ ${binding.replace('.', '\\.')} \\}\\}<\\/text>`), `${modifier} should bind ${binding} inside its own panel`)
}
assert.ok(relationViews.some((tag) => staticClassTokens(tag).includes('drive-pair')), 'relation drives should use a two-column container')
const drivePairBlocks = elementBlocksByStaticClass(relationTemplate, 'view', 'drive-pair')
assert.equal(drivePairBlocks.length, 1, 'relation should render exactly one bounded drive-pair container')
assert.match(drivePairBlocks[0], /\{\{ analysis\.myDrive \}\}/, 'drive-pair should bind my drive inside its own container')
assert.match(drivePairBlocks[0], /\{\{ analysis\.taDrive \}\}/, 'drive-pair should bind TA drive inside its own container')
assert.match(pageStyleDeclarations(relationStyle, '.drive-pair'), /grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\)\s*;/, 'relation drives should remain two equal columns')
assert.match(pageStyleDeclarations(relationStyle, '.insight--bond'), /border-color:\s*#c084fc\s*;/i, 'bond insight should keep its purple connection accent')
assert.match(pageStyleDeclarations(relationStyle, '.insight--friction'), /border-color:\s*#fb7185\s*;/i, 'friction insight should keep its coral accent')
assert.match(pageStyleDeclarations(relationStyle, '.insight--tip'), /border-color:\s*#2dd4bf\s*;/i, 'advice insight should keep its teal accent')
const relationBodyColor = pageStyleDeclarations(relationStyle, '.insight__text')?.match(/color:\s*(#[\da-f]{6})\s*;/i)?.[1]
assert.ok(relationBodyColor, 'relation insight body copy should expose a parseable text color')
for (const { modifier } of insightContracts) {
  const background = pageStyleDeclarations(relationStyle, `.${modifier}`)?.match(/background:\s*linear-gradient\([^,]+,\s*(#[\da-f]{3,6})\s*,\s*(#[\da-f]{3,6})\s*\)\s*;/i)
  assert.ok(background, `${modifier} should expose two parseable gradient background endpoints`)
  for (const endpoint of background.slice(1)) {
    const ratio = contrastRatio(relationBodyColor, endpoint)
    assert.ok(ratio >= 4.5, `${modifier} body text should meet 4.5:1 contrast against ${endpoint}, got ${ratio.toFixed(2)}:1`)
  }
}

for (const { selector, minimum } of [
  { selector: '.relation-hero__eyebrow', minimum: 24 },
  { selector: '.type-picker__step', minimum: 24 },
  { selector: '.type-picker__hint', minimum: 24 },
  { selector: '.type-chip__name', minimum: 24 },
  { selector: '.type-chip__selected', minimum: 24 },
  { selector: '.pair__role', minimum: 24 },
  { selector: '.pair__name', minimum: 24 },
  { selector: '.pair-connection__eyebrow', minimum: 24 },
  { selector: '.pair-connection__label', minimum: 24 },
  { selector: '.insight__eyebrow', minimum: 24 },
  { selector: '.insight__text', minimum: 24 },
  { selector: '.drive__eyebrow', minimum: 24 },
  { selector: '.drive-card__label', minimum: 24 },
  { selector: '.drive-card__text', minimum: 24 },
  { selector: '.disclaimer', minimum: 24 },
]) {
  const fontSizeRule = pageStyleDeclarationBlocks(relationStyle, selector)
    .find((declarations) => /font-size:/.test(declarations))
  const fontSize = fontSizeRule?.match(/font-size:\s*(\d+)rpx\s*;/)
  assert.ok(fontSize && Number(fontSize[1]) >= minimum, `${selector} should keep at least ${minimum}rpx readable text`)
}

assert.doesNotMatch(relationTemplate, /✦|⚡|↗/, 'relation insight icons should not depend on emoji or character glyphs')
for (const modifier of ['bond', 'friction', 'tip']) {
  const marks = elementBlocksByStaticClass(relationTemplate, 'view', `insight__icon--${modifier}`)
  assert.equal(marks.length, 1, `relation should render one CSS icon container for ${modifier}`)
  assert.match(marks[0], new RegExp(`<view\\s+class=["']insight__mark insight__mark--${modifier}["']\\s*\\/>`), `${modifier} insight should render a CSS-only mark`)
  assert.ok(pageStyleDeclarations(relationStyle, `.insight__mark--${modifier}`), `${modifier} insight should define its CSS mark shape`)
}

assert.match(
  resultPage,
  /<!--\s*#ifdef MP-WEIXIN\s*-->[\s\S]*@click="makePoster"[\s\S]*<!--\s*#endif\s*-->/,
  'poster generation must stay limited to mp-weixin',
)
assert.match(
  resultPage,
  /<!--\s*#ifdef H5\s*-->[\s\S]*<button[^>]*disabled[^>]*>小程序内生成海报<\/button>[\s\S]*<!--\s*#endif\s*-->/,
  'H5 poster entry should be a disabled compatibility hint',
)

assert.match(bookingPage, /loadBookingDraft/, 'booking page should restore a local booking draft')
assert.match(bookingPage, /saveBookingDraft/, 'booking page should auto-save booking draft changes')
assert.match(bookingPage, /clearBookingDraft/, 'booking page should clear the local draft after successful submit')

console.log('ui compatibility tests passed')
