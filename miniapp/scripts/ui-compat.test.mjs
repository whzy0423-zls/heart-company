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

function standaloneStyleDeclarations(className) {
  const escapedClassName = className.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return indexPage.match(new RegExp(`^[ \\t]*\\.${escapedClassName}\\s*\\{([^}]*)\\}`, 'm'))?.[1]
}

const energyCards = homeOpeningViews.filter((tag) => staticClassTokens(tag).includes('energy-card'))
assert.equal(energyCards.length, 4, 'home page should render exactly four energy dashboard cards')
for (const card of energyCards) {
  assert.match(card, /\saria-label=["'][^"']+["']/, `energy card should expose a non-empty accessibility label: ${card}`)
  assert.match(card, /\srole=["']button["']/, `energy card should use button semantics: ${card}`)
  assert.match(card, /\shover-class=["']energy-card--pressed["']/, `energy card should expose the shared pressed state: ${card}`)
}

assert.match(indexPage, /function\s+goProfile\s*\(\s*\)/, 'home page should define a profile navigation handler')
assert.match(indexPage, /function\s+goProfile\s*\(\s*\)\s*\{[\s\S]*?uni\.switchTab\s*\(\s*\{\s*url:\s*["']\/pages\/profile\/profile["']\s*\}\s*\)/, 'home profile navigation should switch to the profile tab')

const homeProfileAction = findHomeView('home-nav__profile')
assert.ok(homeProfileAction, 'home page should render a profile action in the top navigation')
assert.match(homeProfileAction, /\srole=["']button["']/, 'home profile action should use button semantics')
assert.match(homeProfileAction, /\saria-label=["']打开我的成长档案["']/, 'home profile action should describe the growth profile destination')

const growthCard = findHomeView('growth-card')
assert.ok(growthCard, 'home page should render a teacher and course growth card')
assert.match(growthCard, /\srole=["']button["']/, 'home growth card should use button semantics')
assert.match(growthCard, /\saria-label=["']打开老师课程与成长内容["']/, 'home growth card should describe the teacher and course destination')

const homeOpeningImages = indexPage.match(/<image\b[^>]*>/g) || []
const heroWheel = homeOpeningImages.find((tag) => staticClassTokens(tag).includes('hero__wheel'))
assert.ok(heroWheel, 'home hero should render the enneagram wheel image')
assert.match(heroWheel, /\sv-if=["']wheelVisible["']/, 'home hero wheel should only render while its image is available')
assert.match(heroWheel, /\s@error=["']hideWheel["']/, 'home hero wheel should hide itself after image errors')
assert.match(heroWheel, /\slazy-load(?:=|\s|>|$)/, 'home hero wheel should lazy-load')
assert.ok(findHomeView('hero__wheel-fallback'), 'home hero should render a wheel fallback view')

for (const className of ['hero__wheel', 'hero__wheel-fallback']) {
  const declarations = standaloneStyleDeclarations(className)
  assert.ok(declarations, `.${className} should have a standalone CSS rule`)
  assert.match(declarations, /\bwidth:\s*\d+rpx\s*;/, `.${className} should have a fixed width`)
  assert.match(declarations, /\bheight:\s*\d+rpx\s*;/, `.${className} should have a fixed height`)
}
assert.match(indexPage, /@media\s*\(prefers-reduced-motion:\s*reduce\)/, 'home page should respect reduced motion preferences')

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
