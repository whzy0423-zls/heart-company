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
for (const token of ['--nx-bg', '--nx-ink', '--nx-blue', '--nx-coral', '--nx-error', '--nx-focus']) {
  assert.match(appleMobileStyle, new RegExp(token), `apple-mobile.css should define ${token}`)
}
assert.match(appleMobileStyle, /--nx-radius-lg:\s*32rpx/, 'the largest global content radius should be 32rpx')
for (const className of ['.nx-page', '.nx-editorial-hero', '.nx-panel', '.nx-media-row', '.nx-quote', '.nx-field', '.nx-empty', '.nx-error']) {
  assert.match(appleMobileStyle, new RegExp(className.replace('.', '\\.') + '\\s*\\{'), `apple-mobile.css should define ${className}`)
}
for (const className of ['.nx-button--primary', '.nx-button--conversion', '.nx-button--secondary', '.nx-button--text']) {
  const escaped = className.replace('.', '\\.')
  assert.match(appleMobileStyle, new RegExp(`${escaped}\\s*\\{[\\s\\S]*?min-height:\\s*(?:8[8-9]|9\\d|[1-9]\\d{2,})rpx`), `${className} should keep at least an 88rpx touch target`)
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
}

for (const file of ['src/pages/relation/relation.vue', 'src/pages/test/test.vue', 'src/pages/learn/learn.vue', 'src/pages/booking/booking.vue']) {
  const source = readFileSync(file, 'utf8')
  assertRootViewClasses(source, file, ['page-stack', 'ios-page', 'ios-safe-bottom'])
}

const indexPage = readFileSync('src/pages/index/index.vue', 'utf8')
const primaryHomeCtaCopies = indexPage.match(/开始测试/g) || []
assert.equal(primaryHomeCtaCopies.length, 1, 'home page should expose exactly one primary CTA copy: 开始测试')
assert.match(indexPage, /<view\s+class=["'][^"']*nx-editorial-hero[^"']*home-editorial-hero[^"']*["']/, 'home page should expose the editorial hero marker')
assert.match(indexPage, /<image\b[^>]*class=["'][^"']*home-hero__image[^"']*["'][^>]*src=["']\/static\/editorial\/home-hero\.webp["'][^>]*>/, 'home hero should use the dedicated editorial artwork')
assert.doesNotMatch(indexPage.match(/<image\b[^>]*class=["'][^"']*home-hero__image[^"']*["'][^>]*>/)?.[0] || '', /lazy-load/, 'above-fold home hero should load eagerly')
assert.match(indexPage, /\.home-hero__art\s*\{[^}]*aspect-ratio:\s*4\s*\/\s*3/, 'home hero frame should preserve the source artwork 4:3 aspect ratio')
assert.doesNotMatch(indexPage, /\.home-hero__art\s*\{[^}]*height:\s*(?:410|450)rpx/, 'home hero frame should not force a conflicting fixed height')
assert.match(indexPage, /import\s+TypeBadge\s+from\s+["'][^"']*type-badge\.vue["']/, 'home page should use the shared type badge component')
assert.match(indexPage, /<TypeBadge\b[^>]*v-for=["']typeId in typeIds["'][^>]*:type-id=["']typeId["'][^>]*>/, 'home page should visibly loop through type badges 1-9')
assert.match(indexPage, /const\s+typeIds\s*=\s*\[1,\s*2,\s*3,\s*4,\s*5,\s*6,\s*7,\s*8,\s*9\]/, 'home page should expose all nine type badge ids')
assert.match(indexPage, /class=["'][^"']*home-entry--relation[^"']*["'][\s\S]*@click=["']goRelation["']/, 'home page should keep a relationship matching entry')
assert.match(indexPage, /class=["'][^"']*home-entry--learn[^"']*["'][\s\S]*@click=["']goLearn["']/, 'home page should keep a learning entry')
assert.match(indexPage, /DEFAULT_COURSEWARE_ITEMS\[0\]/, 'home recommended course should use the fixed local fallback')
assert.match(indexPage, /class=["'][^"']*home-course[^"']*["'][\s\S]*@click=["']goLearn["']/, 'home page should expose a recommended course media entry')
for (const className of ['home-entry--relation', 'home-entry--learn', 'home-course']) {
  assert.match(indexPage, new RegExp(`<button\\b[^>]*class=["'][^"']*${className}[^"']*["'][^>]*>`), `home navigation control ${className} should use native button semantics`)
}
assert.doesNotMatch(indexPage, /aria-pressed=["']false["']/, 'home navigation links should not claim toggle-button semantics')
assert.match(indexPage, /\.home-entry:focus-visible[\s\S]*\.home-course:focus-visible\s*\{[^}]*outline:/, 'home navigation controls should expose a visible keyboard focus state')
assert.match(indexPage, /<button\s+class=["'][^"']*nx-button--primary[^"']*["'][^>]*@click=["']startTest["'][^>]*>\s*开始测试\s*<\/button>/, 'home primary CTA should use the shared 88rpx tap-target class')
assert.doesNotMatch(indexPage, /openChatPage|goChat|问 AI|AI 对话|打开 AI 对话/, 'home page must not expose AI chat entry copy or handlers')

assert.match(indexPage, /class=["'][^"']*home-hero__composition[^"']*["']/, 'home hero should expose a layered editorial composition')
assert.match(indexPage, /\.home-hero__copy\s*\{[^}]*margin-(?:top|bottom):\s*-\d+rpx/, 'home hero copy carrier should visibly overlap the artwork')
assert.match(indexPage, /\.home-hero__art\s*\{[^}]*(?:margin-left:\s*-\d+rpx|width:\s*calc\(100%\s*\+\s*\d+rpx\))/, 'home hero artwork should break into a full-bleed layer')
const decorativeHeroToken = indexPage.match(/<view\b[^>]*class=["'][^"']*home-hero__float-token[^"']*["'][^>]*>/)?.[0] || ''
assert.match(decorativeHeroToken, /aria-hidden=["']true["']/, 'floating hero type token should be purely decorative')
assert.match(indexPage, /\.home-hero__float-token\s*\{[^}]*pointer-events:\s*none/, 'decorative hero type token must not intercept interaction')
assert.match(indexPage, /class=["'][^"']*home-bento[^"']*["']/, 'secondary home actions should share an asymmetric Bento composition')
assert.match(indexPage, /grid-template-areas:\s*["'][^"']*relation[^"']*learn[^"']*["']\s*["'][^"']*relation[^"']*course[^"']*["']/, 'Bento blocks should occupy visibly different grid areas')
assert.match(indexPage, /\.home-entry--relation\s*\{[^}]*grid-area:\s*relation/, 'relationship entry should use the large Bento area')
assert.match(indexPage, /\.home-entry--learn\s*\{[^}]*grid-area:\s*learn/, 'learning entry should use a distinct Bento area')
assert.match(indexPage, /\.home-course-wrap\s*\{[^}]*grid-area:\s*course/, 'course recommendation should use a distinct Bento area')
assert.match(indexPage, /class=["'][^"']*home-explore-band[^"']*["']/, 'explore section should expose a section color-band layer')
assert.match(indexPage, /\.home-explore-band\s*\{[^}]*(?:linear-gradient|repeating-linear-gradient)/, 'section band should use a static CSS paper texture')
assert.match(indexPage, /@media\s+screen\s+and\s+\(min-width:\s*768px\)\s*\{[\s\S]*?\.home-hero__composition\s*\{[^}]*grid-template-columns:[^}]*/, 'tablet home hero should become an intentional two-column composition at 768px')
assert.match(indexPage, /@media\s+screen\s+and\s+\(min-width:\s*768px\)\s*\{[\s\S]*?\.home-bento\s*\{[^}]*grid-template-columns:[^}]*/, 'tablet Bento should become an intentional two-column composition at 768px')
assert.match(indexPage, /transition:\s*(?:opacity|transform)\s+\.(?:18|19|2[0-6])s[^;]*,\s*(?:opacity|transform)\s+\.(?:18|19|2[0-6])s/, 'home press feedback should stay within the 180-260ms opacity/transform motion budget')
assert.match(indexPage, /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{[\s\S]*transition:\s*none/, 'home should disable nonessential motion when reduced motion is requested')
assert.doesNotMatch(indexPage, /(?:backdrop-)?filter\s*:/, 'home must not use filter or backdrop-filter effects')
assert.doesNotMatch(indexPage, /\.home(?:__canvas|-explore-band)[^{]*\{[^}]*animation:\s*[^;}]*infinite/, 'home must not continuously animate full-page or texture layers')
assert.match(indexPage, /\.home-course-wrap\s*\{[^}]*background:\s*#F2E8D0/, 'home course block should keep the approved paper background')
assert.match(indexPage, /\.home-course__edition\s*\{[^}]*color:\s*#59636C/, 'WEEKLY EDIT label should use the tested 5.03:1 foreground on #F2E8D0')
assert.match(indexPage, /\.home-hero__cta\s*\{[^}]*transition:\s*transform\s+\.22s[^;]*,\s*opacity\s+\.22s[^;]*,\s*background-color\s+\.22s[^;]*,\s*box-shadow\s+\.22s/, 'home primary CTA should use a scoped 220ms transform, opacity, background, and shadow press transition')
assert.match(indexPage, /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{[\s\S]*?\.home-hero__cta,[\s\S]*?transition:\s*none/, 'reduced motion should disable the home primary CTA transition')
const homeEntryStyles = indexPage.match(/\.home-entry\s*\{([^}]*)\}/)?.[1] || ''
assert.match(homeEntryStyles, /animation:\s*home-rise\s+\.22s\s+ease-out\s+backwards/, 'home entry animation should stop filling after entry so press transforms can apply')
assert.doesNotMatch(homeEntryStyles, /animation:[^;]*(?:both|forwards)/, 'home entry animation must not persist its final transform or opacity')
assert.match(indexPage, /\.home-entry--pressed,[\s\S]*?\.home-course--pressed\s*\{[^}]*opacity:\s*\.82[^}]*transform:\s*scale\(\.985\)/, 'home entry pressed state should retain visible opacity and scale feedback')
assert.match(indexPage, /\.home-entry--relation\s*\{[^}]*background:\s*#C9472D[^}]*color:\s*#FFFFFF/, 'relationship block should expose the tested coral and white contrast pair')
assert.match(indexPage, /\.home-entry--relation\s+\.home-entry__kicker\s*\{[^}]*color:\s*#FFFFFF[^}]*opacity:\s*1/, 'RELATIONSHIP label should render full-opacity white on #C9472D for 4.75:1 contrast')


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


const bookingPage = readFileSync('src/pages/booking/booking.vue', 'utf8')
assert.match(bookingPage, /userErrorMessage/, 'booking page should surface normalized request errors')
assert.match(bookingPage, /title:\s*userErrorMessage\(e,\s*'提交失败，请重试'\)/, 'booking submit should keep a fallback while showing specific API errors')
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
const startFunction = testPage.match(/function start\(g\) \{[\s\S]*?\n\}/)?.[0] || ''
const chooseFunction = testPage.match(/function choose\(opt\) \{[\s\S]*?\n\}/)?.[0] || ''
assert.match(testPage, /answerLocked/, 'test page should guard rapid repeated option taps')
assert.match(testPage, /clearAdvanceTimer/, 'test page should clear pending navigation timers')
assert.match(testPage, /onUnload/, 'test page should cleanup timers on unload')
assert.match(testPage, /import\s*\{[^}]*nextTick[^}]*\}\s*from\s*["']vue["']/, 'quiz should schedule question focus after rendering')
assert.match(testPage, /const\s+questionHeading\s*=\s*ref\(null\)/, 'quiz should keep a question heading focus target')
assert.match(testPage, /function\s+focusQuestionHeading\(\)\s*\{[\s\S]*nextTick\([\s\S]*#ifdef H5[\s\S]*\.focus\?\.\(\)[\s\S]*#endif[\s\S]*\}/, 'quiz should focus the rendered question on H5 without running DOM focus code on mini program builds')
assert.match(startFunction, /focusQuestionHeading\(\)/, 'starting the quiz should focus and announce the first question')
assert.match(chooseFunction, /step\.value\s*\+=\s*1[\s\S]*focusQuestionHeading\(\)/, 'automatic advance should focus and announce the next question')
assert.match(testPage, /role=["']progressbar["']/, 'quiz should expose a semantic progress indicator')
assert.match(testPage, /进度\s*\{\{\s*step\s*\+\s*1\s*\}\}\s*\/\s*\{\{\s*QUESTIONS\.length\s*\}\}/, 'quiz should show current and total progress text')
assert.match(testPage, /const\s+progress\s*=\s*computed\(\(\)\s*=>\s*\(\(step\.value\s*\+\s*1\)\s*\/\s*QUESTIONS\.length\)\s*\*\s*100\)/, 'quiz progress bar should use the visible current question position')
assert.match(testPage, /:aria-valuenow=["']step\s*\+\s*1["']/, 'quiz semantic progress should match the visible current question position')
assert.match(testPage, /第\s*\{\{\s*step\s*\+\s*1\s*\}\}\s*题/, 'quiz should display the current question number')
const quizQuestionHeading = testPage.match(/<text\b[^>]*class=["'][^"']*quiz__q[^"']*["'][^>]*>/)?.[0] || ''
assert.match(quizQuestionHeading, /ref=["']questionHeading["']/, 'quiz question heading should expose the focus target')
assert.match(quizQuestionHeading, /aria-live=["']polite["']/, 'quiz question heading should announce updates politely')
assert.match(quizQuestionHeading, /aria-atomic=["']true["']/, 'quiz question announcement should be atomic')
assert.match(quizQuestionHeading, /tabindex=["']-1["']/, 'quiz question heading should accept programmatic focus')
assert.match(testPage, /questionVisualCenter\(step\.value\)/, 'quiz illustration should be selected from the current question index')
const quizIllustration = testPage.match(/<image\b[^>]*class=["'][^"']*quiz__illustration[^"']*["'][^>]*>/)?.[0] || ''
assert.match(quizIllustration, /:src=["']questionVisualSrc["']/, 'quiz should render the current center illustration')
assert.doesNotMatch(quizIllustration, /lazy-load/, 'current quiz illustration should load eagerly')
assert.match(testPage, /\.quiz__visual\s*\{[^}]*aspect-ratio:\s*3\s*\/\s*2/, 'quiz illustration should reserve a fixed 3:2 frame')
const quizVisualStyles = testPage.match(/\.quiz__visual\s*\{([^}]*)\}/)?.[1] || ''
const quizVisualMaxWidth = Number(quizVisualStyles.match(/max-width:\s*(\d+)rpx/)?.[1])
assert.ok(quizVisualMaxWidth > 0 && quizVisualMaxWidth <= 280, 'quiz illustration should remain compact auxiliary media at 280rpx wide or less')
assert.match(testPage, /quiz__opt--selected/, 'quiz options should expose a distinct selected state')
assert.match(testPage, /:key=["']step\s*\+\s*["']-["']\s*\+\s*k["']/, 'quiz options should use question-specific keys so focused controls are not silently reused')
assert.match(chooseFunction, /else\s*\{[\s\S]*advanceTimer\s*=\s*setTimeout\(\(\)\s*=>\s*\{[\s\S]*finish\(\)[\s\S]*\},\s*(?:18\d|19\d|2[0-4]\d|250)\s*\)/, 'final answer should stay selected briefly before finishing through the existing timer path')
assert.match(testPage, /\.quiz__back\s*\{[\s\S]*min-height:\s*88rpx/, 'quiz back action should keep an 88rpx touch target')
assert.match(testPage, /<button\b[^>]*class=["'][^"']*nx-button--text[^"']*quiz__back[^"']*["'][^>]*>/, 'quiz previous action should use the weak text-button treatment')
assert.match(testPage, /<button\b[\s\S]*class=["'][^"']*gender__card[^"']*["'][\s\S]*aria-label=/, 'test gender choices should use button semantics with accessibility labels')
assert.match(testPage, /<button\b[\s\S]*class=["'][^"']*quiz__back[^"']*["'][\s\S]*@click=["']back["']/, 'quiz previous action should use button semantics')
assert.match(testPage, /<button\b[\s\S]*v-for=["']\(opt, k\) in q\.options["'][\s\S]*class=["']quiz__opt["'][\s\S]*:aria-label=/, 'quiz options should use button semantics with accessibility labels')
assert.match(testPage, /\.quiz__opt\s*\{[\s\S]*min-height:\s*88rpx/, 'quiz options should keep an 88rpx touch target')
assert.match(testPage, /\.quiz__opt:focus-visible[\s\S]*\.quiz__back:focus-visible\s*\{[^}]*outline:/, 'quiz controls should expose a visible keyboard focus state')
assert.match(testPage, /const\s+currentVisualCenter\s*=\s*computed\(\(\)\s*=>\s*questionVisualCenter\(step\.value\)\)/, 'quiz atmosphere should derive only from the existing question visual center mapping')
assert.match(testPage, /:class=["']\[?[^"']*["']quiz--["']\s*\+\s*currentVisualCenter/, 'quiz should expose a head, heart, or gut presentation class from currentVisualCenter')
for (const center of ['head', 'heart', 'gut']) {
  assert.match(testPage, new RegExp(`\\.quiz--${center}\\s*\\{[^}]*(?:background|--quiz-accent):`), `quiz should define a controlled ${center} atmosphere`)
}
assert.match(testPage, /<text\s+class=["']quiz__idx["'][^>]*>\{\{\s*letter\(k\)\s*\}\}<\/text>/, 'quiz options should expose a visible A/B/C marker')
assert.match(testPage, /class=["']quiz__opt-accent["']\s+aria-hidden=["']true["']/, 'quiz options should expose a decorative left-side structural accent')
assert.match(testPage, /class=["']quiz__check["']\s+aria-hidden=["']true["']/, 'quiz selected options should expose a visible shape/check indicator')
assert.match(testPage, /\.quiz__opt--selected[\s\S]*?\{[^}]*(?:background|background-color):\s*(?!transparent|none)[^;}]+[^}]*box-shadow:/, 'quiz selection should use a distinct fill and ring instead of border alone')
assert.match(testPage, /animation:\s*quiz-enter\s+\.(?:18|19|2[0-6])s\s+[^;]*backwards/, 'quiz question/media/options should enter within the 180-260ms motion budget without persisting final styles')
assert.doesNotMatch(testPage, /animation:\s*quiz-enter[^;]*(?:both|forwards)/, 'quiz entry animation must not persist opacity or transform after entry')
assert.match(testPage, /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{[\s\S]*?animation:\s*none[\s\S]*?transition:\s*none/, 'quiz should disable nonessential motion when reduced motion is requested')
assert.match(testPage, /@media\s+screen\s+and\s+\(min-width:\s*768px\)\s*\{[\s\S]*?\.quiz__body\s*\{[^}]*grid-template-columns:/, 'tablet quiz should become an intentional two-column media/content composition at 768px')
assert.match(testPage, /@media\s+screen\s+and\s+\(min-width:\s*768px\)\s*\{[\s\S]*?\.test\.wrap\s*\{[^}]*max-width:\s*(?:1[2-9]\d{2}|[2-9]\d{3,})rpx[^}]*padding-(?:left|right):/, 'tablet quiz page should override the shared 900rpx wrapper limit with responsive gutters')
assert.doesNotMatch(testPage, /(?:backdrop-)?filter\s*:/, 'quiz must not use filter or backdrop-filter effects')
assert.doesNotMatch(testPage, /\.(?:wrap|quiz)(?:__canvas|__texture)?[^\{]*\{[^}]*animation:\s*[^;}]*infinite/, 'quiz must not continuously animate full-page or texture layers')

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
