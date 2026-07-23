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
assert.match(
  pagesConfig,
  /"path"\s*:\s*"pages\/booking-records\/booking-records"[\s\S]*?"navigationBarTitleText"\s*:\s*"预约记录"/,
  'pages.json should register the appointment records page with its Chinese title',
)

const h5Index = readFileSync('index.html', 'utf8')
assert.match(h5Index, /viewport-fit=cover/, 'H5 viewport meta should enable iOS safe-area env variables')

const appVue = readFileSync('src/App.vue', 'utf8')

const appleMobileStyle = readFileSync('src/styles/apple-mobile.css', 'utf8')
assert.match(appVue, /@import ['"]\.\/styles\/apple-mobile\.css['"];/, 'App.vue should import shared Apple/iOS mobile tokens')
for (const token of ['--nx-bg', '--nx-primary', '--nx-card', '--nx-radius', '--nx-safe-bottom']) {
  assert.match(appleMobileStyle, new RegExp(token), `apple-mobile.css should define ${token}`)
}
for (const className of ['.ios-page', '.ios-card', '.ios-button', '.ios-section', '.ios-safe-bottom']) {
  assert.match(appleMobileStyle, new RegExp(className.replace('.', '\\.') + '\\s*\\{'), `apple-mobile.css should define ${className}`)
}
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
assert.match(testPage, /answerLocked/, 'test page should guard rapid repeated option taps')
assert.match(testPage, /clearAdvanceTimer/, 'test page should clear pending navigation timers')
assert.match(testPage, /onUnload/, 'test page should cleanup timers on unload')
assert.match(testPage, /\.quiz__back\s*\{[\s\S]*min-height:\s*88rpx/, 'quiz back action should keep an 88rpx touch target')
assert.match(testPage, /class=["'][^"']*gender[^"']*card[^"']*ios-card[^"']*["']/, 'test gender card should opt into iOS card styling')
assert.match(testPage, /class=["'][^"']*quiz[^"']*card[^"']*ios-card[^"']*["']/, 'test quiz card should opt into iOS card styling')
assert.match(testPage, /<button\b[\s\S]*class=["'][^"']*gender__card[^"']*["'][\s\S]*aria-label=/, 'test gender choices should use button semantics with accessibility labels')
assert.match(testPage, /<button\b[\s\S]*class=["'][^"']*quiz__back[^"']*["'][\s\S]*@click=["']back["']/, 'quiz previous action should use button semantics')
assert.match(testPage, /<button\b[\s\S]*v-for=["']\(opt, k\) in q\.options["'][\s\S]*class=["']quiz__opt["'][\s\S]*:aria-label=/, 'quiz options should use button semantics with accessibility labels')

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

const bookingRecordsPath = 'src/pages/booking-records/booking-records.vue'
assert.ok(statSync(bookingRecordsPath, { throwIfNoEntry: false })?.isFile(), 'appointment records page should exist')
const bookingRecordsPage = readFileSync(bookingRecordsPath, 'utf8')
assert.match(bookingRecordsPage, /listBookingsApi/, 'appointment records should use the authenticated booking list API')
assert.match(bookingRecordsPage, /getToken/, 'appointment records should validate the current auth token')
assert.match(bookingRecordsPage, /clearToken/, 'appointment records should clear auth after missing or expired authentication')
assert.match(bookingRecordsPage, /clearBookingSession/, 'appointment records should clear token-bound booking state when auth changes')
assert.match(bookingRecordsPage, /setBookingSession\(currentToken,\s*record\)/, 'appointment records should bind the selected record to the current token')
assert.match(bookingRecordsPage, /bookingKindLabel/, 'appointment records should render Chinese booking kinds')
assert.match(bookingRecordsPage, /bookingStatusLabel/, 'appointment records should render Chinese booking statuses')
assert.match(bookingRecordsPage, /maskBookingPhone/, 'appointment records should mask phone numbers')
assert.doesNotMatch(bookingRecordsPage, /\.sort\s*\(/, 'appointment records should preserve the API response order')
assert.match(bookingRecordsPage, /v-if=["']loading["']/, 'appointment records should expose a loading state')
assert.match(bookingRecordsPage, /v-else-if=["']loadError["']/, 'appointment records should expose an error state before empty state')
assert.match(bookingRecordsPage, /v-else-if=["']bookings\.length === 0["']/, 'appointment records should expose an empty state')
assert.match(bookingRecordsPage, /aria-live=["']polite["']/, 'appointment records async state should announce changes politely')
assert.match(
  bookingRecordsPage,
  /<button\s+class=["'][^"']*retry-button[^"']*["'][^>]*tabindex=["']0["'][^>]*@click\.stop=["']retryLoad["'][^>]*>/,
  'appointment records retry should be an independently focusable native button that stops propagation',
)
assert.match(
  bookingRecordsPage,
  /<button\s+class=["'][^"']*empty-action[^"']*["'][^>]*@click=["']goBooking["'][^>]*>去预约<\/button>/,
  'appointment records empty state should switch to the booking tab',
)
assert.match(
  bookingRecordsPage,
  /uni\.switchTab\s*\(\s*\{\s*url:\s*["']\/pages\/booking\/booking["']\s*\}\s*\)/,
  'appointment records empty action should switch to the booking tab',
)

const bookingRecordOpenTags = (bookingRecordsPage.match(/<view\b[^>]*class=["'][^"']*booking-record__open[^"']*["'][^>]*>/g) || [])
assert.ok(bookingRecordOpenTags.length > 0, 'appointment records should render a dedicated navigation body')
for (const tag of bookingRecordOpenTags) {
  assert.match(tag, /\srole=["']button["']/, 'appointment navigation body should use H5 button semantics')
  assert.match(tag, /\saria-role=["']button["']/, 'appointment navigation body should use WeChat button semantics')
  assert.match(tag, /\stabindex=["']0["']/, 'appointment navigation body should participate in keyboard focus order')
  assert.match(tag, /\s@click=["']openBooking\(record\)["']/, 'appointment navigation body should open its record')
  assert.match(tag, /\s@keydown\.enter=["']openBooking\(record\)["']/, 'appointment navigation body should activate with Enter')
  assert.match(tag, /\s@keydown\.space\.prevent=["']openBooking\(record\)["']/, 'appointment navigation body should activate with Space')
}
assert.match(
  bookingRecordsPage,
  /uni\.navigateTo\s*\(\s*\{\s*url:\s*`\/pages\/booking-detail\/booking-detail\?id=\$\{[^}]+\}`\s*\}\s*\)/,
  'appointment navigation should include the selected booking ID in the detail URL',
)

function vueFunctionBody(source, name) {
  const match = new RegExp(`(?:async\\s+)?function\\s+${name}\\s*\\([^)]*\\)\\s*\\{`).exec(source)
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

const retryLoadBody = vueFunctionBody(bookingRecordsPage, 'retryLoad')
assert.ok(retryLoadBody !== undefined, 'appointment records should define an isolated retry handler')
assert.doesNotMatch(retryLoadBody, /setBookingSession|navigateTo|openBooking/, 'retry must never set a booking session or navigate')
const authLossBody = vueFunctionBody(bookingRecordsPage, 'handleAuthLoss')
assert.ok(authLossBody !== undefined, 'appointment records should centralize authentication loss handling')
assert.match(authLossBody, /clearToken\(\)/, 'authentication loss should clear auth')
assert.match(authLossBody, /clearBookingSession\(\)/, 'authentication loss should clear booking session data')
assert.match(authLossBody, /redirecting/, 'authentication loss should guard Toast and navigation side effects')
assert.match(authLossBody, /uni\.showToast/, 'authentication loss should show one user-facing Toast')
assert.match(authLossBody, /uni\.switchTab/, 'authentication loss should switch back to the profile tab')
assert.match(bookingRecordsPage, /loadTicket/, 'appointment records should invalidate stale async responses')
assert.match(bookingRecordsPage, /getToken\(\)\s*!==\s*requestToken/, 'appointment records should reject responses after token changes')
assert.match(bookingRecordsPage, /statusCode\s*===\s*401[\s\S]*statusCode\s*===\s*403/, 'appointment records should handle both 401 and 403')
assert.match(bookingRecordsPage, /onUnload/, 'appointment records should invalidate loads and clear session on unload')
assert.match(bookingRecordsPage, /\.booking-record__open:focus-visible[\s\S]*(?:outline|box-shadow)/, 'appointment navigation should expose a visible focus state')
assert.match(bookingRecordsPage, /\.booking-record__open\s*\{[\s\S]*min-height:\s*88rpx/, 'appointment navigation should keep an 88rpx touch target')


const resultPage = readFileSync('src/pages/result/result.vue', 'utf8')
assert.match(resultPage, /v-else-if="reportError"/, 'result page should render report failure state before falling back to manual fetch')
assert.match(resultPage, /report__retry[\s\S]*@click="loadReportContent"/, 'result page should allow retrying report content fetch from the error state')
assert.match(resultPage, /userErrorMessage/, 'result page should surface normalized request errors')
assert.match(resultPage, /normalizeLastResult/, 'result page should validate cached result schema before rendering')
assert.match(resultPage, /测试结果已失效/, 'result page should give feedback when cached result schema is invalid')

const relationPage = readFileSync('src/pages/relation/relation.vue', 'utf8')
assert.match(relationPage, /<view\s+class=["'][^"']*page-stack[^"']*ios-page[^"']*ios-safe-bottom[^"']*["']/, 'relation root should use shared page-stack/iOS safe-area classes')
assert.match(relationPage, /class=["'][^"']*card[^"']*ios-card[^"']*intro[^"']*["']/, 'relation intro card should opt into iOS card styling')
assert.match(relationPage, /<button\s+class=["'][^"']*btn-primary[^"']*ios-button[^"']*["'][^>]*@click=["']analyze["']/, 'relation primary action should opt into iOS button styling')
assert.doesNotMatch(relationPage, /padding-bottom:\s*60rpx/, 'relation page should not hard-code bottom padding outside shared safe-area helpers')
const relationGridGap = relationPage.match(/\.grid\s*\{[\s\S]*?gap:\s*(\d+)rpx/)
assert.ok(relationGridGap && Number(relationGridGap[1]) >= 16, 'relation type grid gap should be at least 16rpx')
assert.match(relationPage, /isValidTypeId/, 'relation page should validate incoming and selected type ids')
assert.match(relationPage, /stage\.value\s*=\s*'redirecting'/, 'relation invalid query should enter a redirecting state instead of leaving the pick UI interactive')
assert.match(relationPage, /v-else-if="stage === 'result'"/, 'relation result view should be explicit so redirecting can show a safe placeholder')
assert.match(relationPage, /型号参数无效/, 'relation page should explain invalid query type before navigation')
assert.match(relationPage, /\/pages\/test\/test/, 'relation page should return to the test page for invalid query type')
assert.match(relationPage, /\.chip\s*\{[\s\S]*min-height:\s*88rpx/, 'relation type chips should keep an 88rpx touch target')
assert.match(relationPage, /<button\b[\s\S]*v-for=["']t in allTypes["'][\s\S]*class=["']chip["'][\s\S]*:aria-label=/, 'relation type chips should use button semantics with accessibility labels')
assert.match(relationPage, /hover-class=["']chip--hover["']/, 'relation type chips should expose a hover/press visual state')
assert.match(relationPage, /\.chip--hover\s*\{[\s\S]*(?:opacity|transform)/, 'relation chip hover state should have visible feedback')

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
