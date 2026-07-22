import assert from 'node:assert/strict'
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { inflateSync } from 'node:zlib'

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

function decodeRgbaPng(path) {
  const png = readFileSync(path)
  assert.deepEqual(
    png.subarray(0, 8),
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    `${path} should have a valid PNG signature`,
  )
  const width = png.readUInt32BE(16)
  const height = png.readUInt32BE(20)
  assert.equal(png[24], 8, `${path} should use 8-bit channels`)
  assert.equal(png[25], 6, `${path} should preserve RGBA transparency`)
  assert.equal(png[28], 0, `${path} should remain non-interlaced`)

  const idat = []
  for (let offset = 8; offset < png.length;) {
    const length = png.readUInt32BE(offset)
    const type = png.subarray(offset + 4, offset + 8).toString('ascii')
    if (type === 'IDAT') idat.push(png.subarray(offset + 8, offset + 8 + length))
    offset += 12 + length
  }

  const filtered = inflateSync(Buffer.concat(idat))
  const stride = width * 4
  const rgba = Buffer.alloc(stride * height)
  let sourceOffset = 0
  for (let y = 0; y < height; y += 1) {
    const filter = filtered[sourceOffset]
    sourceOffset += 1
    for (let x = 0; x < stride; x += 1) {
      const raw = filtered[sourceOffset + x]
      const left = x >= 4 ? rgba[y * stride + x - 4] : 0
      const up = y > 0 ? rgba[(y - 1) * stride + x] : 0
      const upperLeft = y > 0 && x >= 4 ? rgba[(y - 1) * stride + x - 4] : 0
      let value
      if (filter === 0) value = raw
      else if (filter === 1) value = raw + left
      else if (filter === 2) value = raw + up
      else if (filter === 3) value = raw + Math.floor((left + up) / 2)
      else if (filter === 4) {
        const estimate = left + up - upperLeft
        const leftDistance = Math.abs(estimate - left)
        const upDistance = Math.abs(estimate - up)
        const upperLeftDistance = Math.abs(estimate - upperLeft)
        value = raw + (leftDistance <= upDistance && leftDistance <= upperLeftDistance
          ? left
          : upDistance <= upperLeftDistance ? up : upperLeft)
      } else {
        assert.fail(`${path} uses unsupported PNG filter ${filter}`)
      }
      rgba[y * stride + x] = value & 0xff
    }
    sourceOffset += stride
  }
  return { width, height, rgba }
}

for (const name of ['test', 'learn', 'booking', 'profile']) {
  const path = `src/static/tabbar/${name}-active-green.png`
  assert.equal(existsSync(path), true, `${path} should exist`)
  const { width, height, rgba } = decodeRgbaPng(path)
  assert.deepEqual([width, height], [81, 81], `${path} should retain the existing tab icon dimensions`)
  const original = decodeRgbaPng(`src/static/tabbar/${name}-active.png`)
  assert.deepEqual([original.width, original.height], [width, height], `${path} should match the existing active icon dimensions`)

  const colorCounts = new Map()
  for (let offset = 0; offset < rgba.length; offset += 4) {
    assert.equal(
      rgba[offset + 3],
      original.rgba[offset + 3],
      `${path} should preserve the existing active icon alpha silhouette at pixel ${offset / 4}`,
    )
    if (rgba[offset + 3] === 0) continue
    const color = `${rgba[offset]},${rgba[offset + 1]},${rgba[offset + 2]}`
    colorCounts.set(color, (colorCounts.get(color) || 0) + 1)
    assert.equal(color, '51,91,74', `${path} visible pixel ${offset / 4} should use brand green #335B4A`)
  }
  const dominantColor = [...colorCounts.entries()].sort((a, b) => b[1] - a[1])[0]?.[0]
  assert.equal(dominantColor, '51,91,74', `${path} dominant selected color should be brand green #335B4A`)
  assert.equal(colorCounts.has('43,127,255'), false, `${path} should not retain the old selected blue`)
}

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
const indexTemplate = indexPage.match(/<template>[\s\S]*?<\/template>/)?.[0] || ''
const teacherCoursewareSource = readFileSync('src/utils/teacherCourseware.js', 'utf8')
assert.match(indexPage, /getStoredSiteConfig/, 'home page should render stored site config before refreshing')
assert.match(indexPage, /refreshSiteConfig/, 'home page should refresh site config in the background')
assert.match(indexPage, /normalizeTeachers/, 'home page should normalize teacher data including teacherTeaser')
assert.match(indexPage, /normalizeCoursewareItems/, 'home page should normalize enriched course and material data')
assert.match(indexPage, /const\s+TEACHER_SECTION_PATHS\s*=\s*\[[\s\S]*?teacherTeaser[\s\S]*?\]/, 'home page should explicitly enumerate teacher section sources')
assert.match(indexPage, /const\s+COURSE_SECTION_PATHS\s*=\s*\[[\s\S]*?courseware[\s\S]*?materials[\s\S]*?lessons[\s\S]*?courses[\s\S]*?\]/, 'home page should explicitly enumerate course section sources')
assert.match(indexPage, /Object\.prototype\.hasOwnProperty\.call/, 'home section detection should distinguish missing fields from explicit empty fields')
assert.match(indexPage, /function\s+hasTeacherSection\(config\)/, 'home page should expose teacher section-presence detection')
assert.match(indexPage, /function\s+hasCourseSection\(config\)/, 'home page should expose course section-presence detection')
assert.match(indexPage, /if\s*\(!preserveMissing\s*\|\|\s*hasTeacherSection\(config\)\)\s*\{[\s\S]*?teachers\.value\s*=\s*normalizeTeachers\(config\)/, 'partial refreshes missing teacher fields should preserve the currently displayed teacher')
assert.match(indexPage, /if\s*\(!preserveMissing\s*\|\|\s*hasCourseSection\(config\)\)\s*\{[\s\S]*?courses\.value\s*=\s*normalizeCoursewareItems\(config\)/, 'partial refreshes missing course fields should preserve the currently displayed courses')
assert.match(indexPage, /applyContent\(config,\s*\{\s*preserveMissing:\s*true\s*\}\)/, 'successful background refresh should merge only explicitly present teacher/course sections')
assert.match(indexPage, /let\s+loadTicket\s*=\s*0/, 'home page should guard against stale refresh responses')
assert.match(indexPage, /ticket\s*!==\s*loadTicket/, 'home page should ignore a stale refresh response')
assert.match(indexPage, /v-if=["']loading["']/, 'home page should expose an explicit loading state')
assert.match(indexPage, /v-if=["']loadError["']/, 'home page should expose a non-blocking refresh error')
assert.match(indexPage, /@click=["']activateAction\(loadContent,\s*\$event\)["']/, 'home refresh failure should provide a cross-platform retry action')
assert.match(indexPage, /资料整理中/, 'explicit empty teacher or course sections should render a local empty state')

const teacherHeroImage = indexPage.match(/<image\b[^>]*class=["'][^"']*teacher-hero__image[^"']*["'][^>]*>/)?.[0] || ''
assert.match(teacherHeroImage, /:src=["']teacherImage["']/, 'teacher portrait should use a resolved render source')
assert.match(teacherHeroImage, /role=["']img["']/, 'teacher image host should expose an img role on H5')
assert.match(teacherHeroImage, /:aria-label=["']teacherImageLabel["']/, 'teacher portrait should expose a meaningful accessible label')
assert.match(teacherHeroImage, /@error=["']onTeacherImageError["']/, 'teacher portrait should provide a local image fallback')
assert.doesNotMatch(teacherHeroImage, /lazy-load/, 'dominant above-fold teacher portrait should load eagerly')
assert.match(indexPage, /resolveContentAsset\(teacher\.value\?\.avatar,\s*TEACHER_FALLBACK\)/, 'teacher image should resolve backend content before rendering')
assert.match(indexPage, /teacherImageFallbackUsed/, 'teacher fallback should be applied only once')
assert.match(indexPage, /teacher\.name/, 'teacher hero should render the teacher name')
assert.match(indexPage, /teacher\.title/, 'teacher hero should render teacher identity')
assert.match(indexPage, /teacher\.bio/, 'teacher hero should render a concise credibility biography')
assert.match(indexPage, /function\s+activateAction\(action,\s*event\)\s*\{[\s\S]*?event\?\.repeat[\s\S]*?keyboardActivationAt[\s\S]*?action\(\)/, 'home keyboard activation should ignore repeats and suppress the synthetic follow-up click')
assert.match(indexPage, /function\s+onActionKeydown\(event,\s*action\)\s*\{\s*if\s*\(!\['Enter',\s*' ',\s*'Spacebar'\]\.includes\(event\?\.key\)\)\s*return\s*event\.preventDefault\?\.\(\)\s*event\.stopPropagation\?\.\(\)\s*activateAction\(action,\s*event\)/, 'the shared keydown handler should prevent only Enter/Space after filtering, leaving Tab untouched')

const teacherHeroImages = indexPage.match(/<image\b[^>]*class=["'][^"']*teacher-hero__image[^"']*["'][^>]*>/g) || []
assert.equal(teacherHeroImages.length, 1, 'home page should render exactly one teacher hero image')
assert.match(indexPage, /class=["'][^"']*teacher-welcome[^"']*["']/, 'home should use a compact teacher welcome section')
const teacherToggle = indexPage.match(/<view\b[^>]*class=["'][^"']*teacher-toggle[^"']*["'][^>]*>/)?.[0] || ''
assert.match(teacherToggle, /role=["']button["']/, 'teacher intro toggle should expose button semantics')
assert.match(teacherToggle, /tabindex=["']0["']/, 'teacher intro toggle should be keyboard focusable')
assert.match(teacherToggle, /:aria-expanded=["']teacherExpanded["']/, 'teacher intro toggle should expose expanded state')
assert.match(teacherToggle, /aria-controls=["']teacher-bio["']/, 'teacher intro toggle should identify the controlled biography')
assert.match(teacherToggle, /@click=["']activateAction\(toggleTeacher,\s*\$event\)["']/, 'teacher intro toggle should support click and mini-program tap')
assert.match(teacherToggle, /@keydown=["']onActionKeydown\(\$event,\s*toggleTeacher\)["']/, 'teacher intro toggle should support Enter and Space')
assert.match(indexPage, /\.teacher-toggle\s*\{[^}]*min-height:\s*88rpx/, 'teacher intro toggle should keep an 88rpx touch target')

const serviceEntries = indexPage.match(/<view\b[^>]*class=["'][^"']*service-entry[^"']*["'][^>]*>/g) || []
assert.equal(serviceEntries.length, 4, 'home should expose exactly four primary service entries')
for (const copy of ['课程学习', '课件资料', '九型测试', '关系合盘']) {
  assert.equal((indexPage.match(new RegExp(copy, 'g')) || []).length, 1, `home should expose one ${copy} service entry`)
}
for (const action of serviceEntries) {
  assert.match(action, /role=["']button["']/, 'service entries should expose button semantics')
  assert.match(action, /tabindex=["']0["']/, 'service entries should be keyboard focusable')
  assert.match(action, /@click=["']activateAction\(/, 'service entries should preserve tap and click activation')
  assert.match(action, /@keydown=["']onActionKeydown\(/, 'service entries should support Enter and Space')
}

function functionSource(source, name, nextName) {
  const start = source.indexOf(`function ${name}(`)
  const end = source.indexOf(`function ${nextName}(`, start)
  assert.notEqual(start, -1, `${name} should exist`)
  assert.notEqual(end, -1, `${nextName} should follow ${name}`)
  return source.slice(start, end)
}

const goCourseSource = functionSource(indexPage, 'goCourse', 'goMaterial')
const goMaterialSource = functionSource(indexPage, 'goMaterial', 'startTest')
for (const [source, intent, label] of [
  [goCourseSource, 'course', 'course'],
  [goMaterialSource, 'material', 'material'],
]) {
  assert.match(source, new RegExp(`setLearningNavIntent\\(['"]${intent}['"]\\)`), `${label} navigation should set its short-lived intent`)
  assert.match(source, /uni\.switchTab\(\{[\s\S]*?url:\s*['"]\/pages\/learn\/learn['"]/, `${label} navigation should switch to the learning tab`)
  assert.match(source, /fail\(\)\s*\{\s*clearLearningNavIntent\(\)\s*\}/, `${label} navigation should clear its intent if switchTab fails`)
}
assert.match(indexPage, /uni\.navigateTo\(\{\s*url:\s*['"]\/pages\/test\/test['"]/, 'test service should navigate to the test page')
assert.match(indexPage, /uni\.navigateTo\(\{\s*url:\s*['"]\/pages\/relation\/relation['"]/, 'relation service should navigate to the relation page')

const featuredCourseActions = indexPage.match(/<view\b[^>]*class=["']featured-course["'][^>]*>/g) || []
assert.equal(featuredCourseActions.length, 1, 'home should render exactly one recommended course row')
assert.equal((indexPage.match(/推荐课程/g) || []).length, 1, 'home should label the recommended course section once')
const moreCoursesAction = indexPage.match(/<view\b[^>]*class=["'][^"']*section-link--course[^"']*["'][^>]*>/)?.[0] || ''
assert.match(moreCoursesAction, /role=["']button["']/, 'more courses should be an accessible action')
assert.match(moreCoursesAction, /tabindex=["']0["']/, 'more courses should be keyboard focusable')
assert.match(moreCoursesAction, /@click=["']activateAction\(goCourse,\s*\$event\)["']/, 'more courses should open the course category')
assert.match(moreCoursesAction, /@keydown=["']onActionKeydown\(\$event,\s*goCourse\)["']/, 'more courses should support Enter and Space')
assert.doesNotMatch(indexPage, /material-shelf|material-card|v-for=["']\(course,\s*index\) in materialCourses/, 'home page should not render a course shelf or additional course list')
const courseCoverImages = indexPage.match(/<image\b[^>]*class=["'][^"']*featured-course__cover[^"']*["'][^>]*>/g) || []
assert.equal(courseCoverImages.length, 1, 'home page should render exactly one featured course cover')
for (const image of courseCoverImages) {
  assert.match(image, /aria-hidden=["']true["']/, 'course covers should be hidden from assistive technology because titles are adjacent')
  assert.match(image, /@error=["']onCourseImageError\(/, 'course covers should provide a one-shot local editorial fallback')
}
assert.match(indexPage, /function\s+homeCourseCover\(course,\s*index\)/, 'home page should map unsuitable local course covers before rendering')
assert.match(teacherCoursewareSource, /DEFAULT_COURSEWARE_ITEMS[\s\S]*?cover:\s*['"]\/static\/wheel\.png['"]/, 'the compatibility test should cover the current DEFAULT_COURSEWARE_ITEMS wheel fallback')
assert.ok(indexPage.includes('\\/static\\/wheel\\.png'), 'home course cover mapping should explicitly recognize the legacy wheel fallback')
assert.match(indexPage, /return\s+!cover\s*\|\|\s*isLegacyWheel\s*\?\s*courseFallback\(index\)\s*:\s*cover/, 'missing and legacy wheel covers should use distinct editorial publication covers')
assert.match(indexPage, /resolveContentAsset\(homeCourseCover\(course,\s*index\),\s*courseFallback\(index\)\)/, 'featured and shelf course images should resolve the mapped publication cover')
assert.doesNotMatch(indexPage, /resolveContentAsset\(course\.cover,/, 'DEFAULT_COURSEWARE_ITEMS wheel covers must not render directly on home')
assert.match(indexPage, /courseImageFallbackUsed/, 'course cover fallback should be applied only once per item')
assert.equal((indexPage.match(/最新课件/g) || []).length, 1, 'home should expose one latest material section')
assert.match(indexPage, /class=["'][^"']*latest-material[^"']*["']/, 'home should render a latest material row')
const allMaterialsAction = indexPage.match(/<view\b[^>]*class=["'][^"']*section-link--material[^"']*["'][^>]*>/)?.[0] || ''
assert.match(allMaterialsAction, /role=["']button["']/, 'all materials should be an accessible action')
assert.match(allMaterialsAction, /tabindex=["']0["']/, 'all materials should be keyboard focusable')
assert.match(allMaterialsAction, /@click=["']activateAction\(goMaterial,\s*\$event\)["']/, 'all materials should open the material category')
assert.match(allMaterialsAction, /@keydown=["']onActionKeydown\(\$event,\s*goMaterial\)["']/, 'all materials should support Enter and Space')
assert.equal((indexPage.match(/预约咨询/g) || []).length, 1, 'home should expose one lightweight booking prompt')
assert.match(indexPage, /class=["'][^"']*booking-prompt[^"']*["']/, 'home should render booking as a lightweight prompt')
assert.match(indexPage, /function\s+goBooking\(\)\s*\{\s*uni\.switchTab\(\{\s*url:\s*['"]\/pages\/booking\/booking['"]\s*\}\)\s*\}/, 'home consultation entry should switch to the booking tab')
assert.match(indexPage, /@click=["']activateAction\(goBooking,\s*\$event\)["']/, 'home consultation entry should activate booking on click or tap')
assert.match(indexPage, /@keydown=["']onActionKeydown\(\$event,\s*goBooking\)["']/, 'home consultation entry should activate booking from Enter or Space')
const roleButtonViews = indexPage.match(/<view\b(?=[^>]*role=["']button["'])[^>]*>/g) || []
for (const action of roleButtonViews) {
  assert.equal((action.match(/@keydown=/g) || []).length, 1, 'each home action should declare exactly one keydown binding')
}
assert.doesNotMatch(indexPage, /@keydown\./, 'keydown modifiers must not intercept Tab or compile duplicate WXML attributes')
assert.match(indexPage, /\.service-entry\s*\{[^}]*min-height:\s*(?:8[8-9]|9\d|[1-9]\d{2,})rpx/, 'service entries should keep at least an 88rpx touch target')
assert.match(indexPage, /\.service-entry:focus-visible[\s\S]*\.booking-prompt:focus-visible[\s\S]*outline:/, 'home controls should expose a visible keyboard focus state')
assert.match(indexPage, /--home-bg:\s*#F5F6F4/i, 'home should use a light WeUI-like page background')
assert.match(indexPage, /--home-surface:\s*#FFFFFF/i, 'home should use white service blocks')
assert.match(indexPage, /--home-ink:\s*#20252B/i, 'home should use the approved restrained ink color')
assert.match(indexPage, /--home-green:\s*#335B4A/i, 'home should use the approved restrained green accent')
assert.doesNotMatch(indexTemplate, /home-primary|开始学习|teacher-hero__portrait|home-masthead|editorial-kicker|portrait-mark|featured-course__spine|material-shelf|material-card|home-bento|float-token|texture|pattern|CURRICULUM|FEATURED|TEACHER|SELF TEST|RELATIONSHIP|学习专刊/, 'home should omit the old long-page hero, main button, and decorative promotional layers')
assert.doesNotMatch(indexPage, /(?:repeating-)?(?:linear|radial)-gradient|background-image|box-shadow|@keyframes|animation\s*:|backdrop-filter|filter\s*:/, 'home should use a plain background with no texture, normal shadow, filter, or entry animation')
assert.doesNotMatch(indexPage, /border-radius:\s*(?:[3-9]\d|\d{3,})rpx/, 'home radii should stay within the restrained 16-24rpx range')


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
const learningPageStateSource = readFileSync('src/utils/learningPageState.js', 'utf8')

assert.match(learningPageStateSource, /normalizeTeachers/, 'learn state utility should normalize teacher profile data from site config')
assert.match(learningPageStateSource, /normalizeCoursewareItems/, 'learn state utility should normalize courseware and course data from site config')
assert.match(learnPage, /resolveContentAsset/, 'learn page should resolve backend teacher and course image assets safely')
const learnTemplate = learnPage.match(/<template>[\s\S]*?<\/template>/)?.[0] || ''
const learnTeacherSections = learnTemplate.match(/<section\b[^>]*class=["'][^"']*learn-teacher[^"']*["'][^>]*>/g) || []
assert.equal(learnTeacherSections.length, 1, 'learn page should render exactly one compact teacher introduction section')
assert.match(learnPage, /主讲老师/, 'learn page should introduce the teacher in concise Chinese copy')
assert.match(learnPage, /teacher\.name/, 'teacher profile should render the teacher name')
assert.match(learnPage, /teacher\.title/, 'teacher profile should render the teacher title')
assert.match(learnPage, /teacher\.bio/, 'teacher profile should render the teacher biography')
assert.match(learnPage, /teacher\.tags/, 'teacher profile should render teacher expertise tags')

const learnTeacherImage = learnPage.match(/<image\b[^>]*class=["'][^"']*learn-teacher__image[^"']*["'][^>]*>/)?.[0] || ''
assert.match(learnTeacherImage, /:src=["']teacherImage["']/, 'teacher portrait should use a resolved render source')
assert.match(learnTeacherImage, /role=["']img["']/, 'teacher portrait should expose image semantics on H5')
assert.match(learnTeacherImage, /:aria-label=["']teacherImageLabel["']/, 'teacher portrait should expose a meaningful accessible label')
assert.match(learnTeacherImage, /@error=["']onTeacherImageError["']/, 'teacher portrait should provide a local fallback')
assert.doesNotMatch(learnTeacherImage, /lazy-load/, 'dominant teacher portrait should load eagerly')
assert.match(learnPage, /resolveContentAsset\(teacher\.value\?\.avatar,\s*TEACHER_FALLBACK\)/, 'teacher portrait should resolve backend content before rendering')
assert.match(learnPage, /teacherImageFallbackUsed/, 'teacher portrait fallback should be applied only once')
assert.match(learnPage, /\.learn-teacher__portrait\s*\{[^}]*aspect-ratio:\s*4\s*\/\s*5/, 'teacher portrait should reserve an editorial 4:5 frame')

assert.match(learnPage, /class=["'][^"']*course-list[^"']*["']/, 'learn page should render a simple course media list')
assert.match(learnPage, /class=["'][^"']*course-row[^"']*["']/, 'learn page should render courses as lightweight media rows')
assert.match(learnPage, /course\.materialTypes/, 'course rows should expose material types')
assert.match(learnPage, /course\.duration/, 'course rows should expose duration metadata')
assert.match(learnPage, /course\.description/, 'course rows should expose a short description')
assert.doesNotMatch(learnTemplate, /course\.bullets|bulletIndex|::bullet::/, 'course bullet data should not be rendered on the minimal learning page')
const learnCourseCovers = learnPage.match(/<image\b[^>]*class=["'][^"']*course-row__cover[^"']*["'][^>]*>/g) || []
assert.ok(learnCourseCovers.length >= 1, 'course rows should keep one compact cover visual')
for (const image of learnCourseCovers) {
  assert.match(image, /aria-hidden=["']true["']/, 'course covers should be decorative because the title is adjacent')
  assert.match(image, /@error=["']onCourseImageError\(/, 'course covers should provide a one-shot local fallback')
}
assert.match(learnPage, /function\s+learnCourseCover\(course,\s*index\)/, 'learn page should map unsuitable legacy covers before rendering')
assert.ok(learnPage.includes('\\/static\\/wheel\\.png'), 'learn page should recognize the legacy wheel cover')
assert.match(learnPage, /resolveContentAsset\(learnCourseCover\(course,\s*index\),\s*courseFallback\(index\)\)/, 'course covers should resolve mapped content assets')
assert.match(learnPage, /courseImageFallbackUsed/, 'course cover fallback should be applied only once per item')
assert.doesNotMatch(learnPage, /class=["'][^"']*course-row[^"']*["'][^>]*role=["']button["']/, 'course rows should remain display-only until a course route exists')

assert.match(learnPage, /class=["'][^"']*pull-quote[^"']*["']/, 'quotes should render as a typographic editorial pull quote')
assert.match(learnTemplate, /v-if=["']quotes\.length["'][\s\S]*?quotes\[0\]/, 'learn page should render only the first quote when available')
assert.doesNotMatch(learnTemplate, /v-for=["'][^"']*quote/, 'learn page should not render a quote stack')
assert.match(learnTemplate, /v-else[^>]*class=["'][^"']*quote-empty/, 'learn page should render an explicit quote empty state')
assert.match(learnPage, /class=["'][^"']*type-index[^"']*["']/, 'the type index should remain available late in the page')
assert.match(learnPage, /\.learn-teacher__bio\s*\{[^}]*font-size:\s*(?:2[6-9]|[3-9]\d|\d{3,})rpx/, 'teacher biography should remain readable at 26rpx or larger')
assert.match(learnPage, /\.course-row__desc\s*\{[^}]*font-size:\s*(?:2[6-9]|[3-9]\d|\d{3,})rpx/, 'course descriptions should remain readable at 26rpx or larger')
assert.match(learnPage, /\.material-types text\s*\{[^}]*font-size:\s*(?:2[2-9]|[3-9]\d|\d{3,})rpx/, 'material metadata should remain readable at 22rpx or larger')
assert.match(learnPage, /\.course-row__duration\s*\{[^}]*font-size:\s*(?:2[2-9]|[3-9]\d|\d{3,})rpx/, 'duration metadata should remain readable at 22rpx or larger')
assert.match(learnPage, /\.type-index__item\s*\{[^}]*font-size:\s*(?:2[2-9]|[3-9]\d|\d{3,})rpx/, 'type index text should remain readable at 22rpx or larger')
assert.doesNotMatch(learnPage, /font-size:\s*19rpx/, 'learn page should not use 19rpx content text')
assert.doesNotMatch(learnPage, /\.type-index__item\s*\{[^}]*min-height:\s*(?:8[8-9]|9\d|[1-9]\d{2,})rpx/, 'display-only type index chips should not become dominant touch cards')
assert.doesNotMatch(learnPage, /class=["'][^"']*ios-card[^"']*["']/, 'learn editorial sections should not fall back to generic ios-card surfaces')
assert.doesNotMatch(learnTemplate, /learn-masthead|publication-section|publication-card|老师专访|PROFILE|PERSONAL VOICE|COURSEWARE PUBLICATION|REFERENCE|TEACHER'S NOTE|馆藏资料|0\{\{\s*index/, 'learn template should omit the complex publication shelf, chapter numbering, and English labels')
assert.doesNotMatch(learnPage, /(?:repeating-)?(?:linear|radial)-gradient|background-image|box-shadow|@keyframes|animation\s*:|(?:backdrop-)?filter\s*:/, 'learn page should use a plain background without texture, gradients, shadows, filters, or animation')
assert.doesNotMatch(learnPage, /border-radius:\s*(?:[3-9]\d|\d{3,})rpx/, 'learn radii should stay within the restrained 16-24rpx range')
assert.match(learnPage, /--learn-bg:\s*#F6F1E7/i, 'learn page should use the approved warm plain background')
assert.match(learnPage, /--learn-surface:\s*#FFFDF8/i, 'learn page should use a warm white content surface')
assert.match(learnPage, /--learn-ink:\s*#20252B/i, 'learn page should use restrained charcoal text')
assert.match(learnPage, /--learn-green:\s*#335B4A/i, 'learn page should use restrained dark green accents')

assert.match(indexPage, /老师|导师/, 'home page should emphasize teacher guidance')
assert.match(indexPage, /课件|课程/, 'home page should emphasize courseware and courses')
assert.doesNotMatch(indexPage, /AI 对话/, 'home page primary feature cards should avoid AI-heavy copy')

assert.match(learnPage, /loadError/, 'learn page should expose a non-blocking failure state')
assert.match(learnPage, /v-if=["']loading["']/, 'learn page should show a loading cue while retaining local fallback content')
assert.match(learnPage, /资料整理中/, 'explicit empty teacher or course sections should render an editorial empty state')
assert.match(learnPage, /createInitialLearningContent\(\)/, 'initial uncached render should use tested local learning fallbacks')
assert.match(learningPageStateSource, /const\s+TEACHER_SECTION_PATHS\s*=\s*\[[\s\S]*?teacherTeaser[\s\S]*?\]/, 'learn state should enumerate all teacher section sources')
assert.match(learningPageStateSource, /const\s+COURSE_SECTION_PATHS\s*=\s*\[[\s\S]*?courseware[\s\S]*?materials[\s\S]*?lessons[\s\S]*?courses[\s\S]*?\]/, 'learn state should enumerate course and material sources')
assert.match(learningPageStateSource, /Object\.prototype\.hasOwnProperty\.call/, 'learn section detection should distinguish missing fields from explicit empty fields')
assert.match(learnPage, /applyLearningContent\(/, 'learn page should apply content through the behavior-tested state utility')
assert.match(learnPage, /applyContent\(config,\s*\{\s*preserveMissing:\s*true\s*\}\)/, 'successful refresh should merge only explicitly present learning sections')
assert.match(learnPage, /createLatestRequestGuard\(\)/, 'learn page should use the tested latest-request guard')
assert.match(learnPage, /requestGuard\.isLatest\(ticket\)/, 'learn page should ignore stale refresh responses')
assert.match(learnPage, /retainLearningContentOnError\(/, 'learn request failures should preserve current content through the tested state utility')
assert.doesNotMatch(learnPage, /teachers\.value\s*=\s*normalizeTeachers\(\)[\s\S]*coursewareItems\.value\s*=\s*normalizeCoursewareItems\(\)[\s\S]*loadError\.value/, 'request failure should not replace currently visible content')
assert.match(learnPage, /id=["']learn-teacher-heading["']\s+class=["']editorial-empty__title["']/, 'empty teacher state should keep the aria-labelledby target available')
for (const keyKind of ['course', 'tag', 'material']) {
  assert.match(learnPage, new RegExp(`::${keyKind}::|${keyKind}::`), `learn ${keyKind} keys should include stable delimiters and indices`)
}

const learnRetry = learnPage.match(/<view\b[^>]*class=["'][^"']*learn-retry[^"']*["'][^>]*>/)?.[0] || ''
assert.match(learnRetry, /role=["']button["']/, 'learn retry should expose button semantics on H5')
assert.match(learnRetry, /tabindex=["']0["']/, 'learn retry should be keyboard focusable on H5')
assert.match(learnRetry, /@click=["']activateAction\(loadContent,\s*\$event\)["']/, 'learn retry should preserve mini-program tap and H5 click activation')
assert.match(learnRetry, /@keydown=["']onActionKeydown\(\$event,\s*loadContent\)["']/, 'learn retry should use one plain keydown handler')
const learnPrimaryAction = learnPage.match(/<view\b[^>]*class=["'][^"']*learn-primary[^"']*["'][^>]*>/)?.[0] || ''
assert.match(learnPrimaryAction, /role=["']button["']/, 'learn primary action should expose button semantics on H5')
assert.match(learnPrimaryAction, /tabindex=["']0["']/, 'learn primary action should be keyboard focusable on H5')
assert.match(learnPrimaryAction, /@click=["']activateAction\(goTest,\s*\$event\)["']/, 'learn primary action should guide to the existing test')
assert.match(learnPrimaryAction, /@keydown=["']onActionKeydown\(\$event,\s*goTest\)["']/, 'learn primary action should use one plain keydown handler')
assert.match(learnPage, /handleActionKeydown\(event,\s*\(\)\s*=>\s*activateAction\(action,\s*event\)\)/, 'learn keydown handler should delegate to the behavior-tested activation filter')
const learnRoleButtons = learnPage.match(/<view\b(?=[^>]*role=["']button["'])[^>]*>/g) || []
for (const action of learnRoleButtons) {
  assert.equal((action.match(/@keydown=/g) || []).length, 1, 'each learn action should declare exactly one keydown binding')
}
assert.doesNotMatch(learnPage, /@keydown\./, 'learn keydown modifiers must not compile duplicate WXML attributes or intercept Tab')
assert.match(learnPage, /\.learn-retry\s*\{[^}]*min-height:\s*88rpx/, 'learn retry should keep an 88rpx touch target')
assert.match(learnPage, /\.learn-primary\s*\{[^}]*min-height:\s*88rpx/, 'learn primary action should keep an 88rpx touch target')
assert.match(learnPage, /\.learn-retry:focus-visible[\s\S]*\.learn-primary:focus-visible[\s\S]*outline:/, 'learn controls should expose a visible keyboard focus state')
assert.match(learnPage, /@media\s+screen\s+and\s+\(min-width:\s*768px\)\s*\{[\s\S]*?\.learn-teacher\s*\{[^}]*grid-template-columns:/, 'tablet teacher section should become a two-column editorial composition')
const learnTabletMedia = learnPage.match(/@media\s+screen\s+and\s+\(min-width:\s*768px\)\s*\{([\s\S]*?)\n\}/)?.[1] || ''
assert.doesNotMatch(learnTabletMedia, /\.course-list\s*\{[^}]*grid-template-columns:\s*repeat\(2/, 'tablet courses should remain a readable single-column list')
assert.doesNotMatch(learnPage, /\.course-list\s*\{[^}]*grid-template-columns:\s*repeat\(2/, 'courses should remain a readable single-column list at every viewport width')
assert.match(learnTabletMedia, /\.learn-primary\s*\{[^}]*width:\s*100%[^}]*max-width:\s*360rpx[^}]*min-width:\s*0[^}]*box-sizing:\s*border-box/, 'tablet teacher CTA should fit its grid column without rpx overflow')

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
