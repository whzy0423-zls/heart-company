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

const tabEntries = JSON.parse(pagesConfig).tabBar.list
for (const tab of tabEntries) {
  const inactive = decodeRgbaPng(`src/${tab.iconPath}`)
  const active = decodeRgbaPng(`src/${tab.selectedIconPath}`)
  assert.deepEqual([inactive.width, inactive.height], [81, 81], `${tab.text} tab icon should keep native icon dimensions`)
  assert.deepEqual([active.width, active.height], [81, 81], `${tab.text} selected icon should keep native icon dimensions`)
  for (const [image, expected, label] of [[inactive, '133,135,125', 'inactive'], [active, '165,92,59', 'selected']]) {
    let visible = 0
    let transparent = 0
    for (let offset = 0; offset < image.rgba.length; offset += 4) {
      if (!image.rgba[offset + 3]) { transparent += 1; continue }
      visible += 1
      assert.equal(`${image.rgba[offset]},${image.rgba[offset + 1]},${image.rgba[offset + 2]}`, expected, `${tab.text} ${label} icon should use the warm navigation palette`)
    }
    assert.ok(visible > 0 && transparent > 0, `${tab.text} ${label} icon should preserve a visible transparent silhouette`)
  }
  for (let offset = 3; offset < active.rgba.length; offset += 4) {
    assert.equal(active.rgba[offset], inactive.rgba[offset], `${tab.text} active/inactive icons should preserve their silhouette`)
  }
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

for (const file of ['src/pages/result/result.vue', 'src/pages/profile/profile.vue']) {
  const source = readFileSync(file, 'utf8')
  assert.match(source, /ios-page/, `${file} should opt into shared Apple/iOS page styling`)
}

for (const file of ['src/pages/relation/relation.vue', 'src/pages/test/test.vue', 'src/pages/learn/learn.vue']) {
  const source = readFileSync(file, 'utf8')
  assertRootViewClasses(source, file, ['page-stack', 'ios-page', 'ios-safe-bottom'])
}

// The teacher studio home uses native buttons and a dedicated teacher detail route.
// Keep functional contracts here; layout order and retired class names are not API contracts.
const indexPage = readFileSync('src/pages/index/index.vue', 'utf8')
const indexTemplate = indexPage.match(/<template>[\s\S]*?<\/template>/)?.[0] || ''
assert.match(indexPage, /getStoredSiteConfig/, 'home should render cached teacher content')
assert.match(indexPage, /refreshSiteConfig/, 'home should refresh public content')
assert.match(indexPage, /normalizeTeachers/, 'home should honor configured teacher fields and explicit empty sections')
assert.match(indexPage, /listClassroomRecentApi/, 'home videos should come from the published classroom API')
assert.match(indexPage, /const videos = ref\(\[\]\)/, 'home must not initialize unpublished video fixtures')
assert.match(indexPage, /if \(current !== ticket\) return/, 'home should discard stale refresh results')
assert.match(indexPage, /Promise\.allSettled/, 'teacher and video requests should fail independently')
assert.match(indexPage, /UI_PREVIEW \? STUDIO_COURSES : normalizeCoursewareItems/, 'simulated course schedules must remain restricted to preview mode')
assert.match(indexTemplate, /v-if="UI_PREVIEW"[^>]*>演示排期/, 'sample schedules should be visibly identified')
assert.match(indexPage, /classroomContentRoute\(item\)/, 'published videos should use the validated classroom detail route')
assert.match(indexPage, /function daily\(\)[\s\S]*?uni\.switchTab\(\{ url: '\/pages\/learn\/learn'/, 'daily sharing should navigate to the native learning tab')
assert.match(indexPage, /setBookingIntent\(\{ kind, intentText: '' \}\)/, 'home booking should carry the selected service type')
assert.match(indexTemplate, /@click="booking\('consult'\)"/, 'consultation should choose consultation rather than the default course type')
assert.match(indexTemplate, /@click="navigate\('\/pages\/teacher\/teacher'\)"/, 'teacher introduction should open the dedicated details page')
assert.match(indexTemplate, /@click="navigate\('\/pages\/test\/test'\)"/, 'personality exploration should keep its test route')
assert.match(indexTemplate, /loading && !dailyVideos\.length/, 'home should show loading only before video content exists')
assert.match(indexTemplate, /v-else-if="error"/, 'video fetch failures should remain distinct from an empty library')
assert.match(indexTemplate, /@click="load">重新加载/, 'video errors should expose a native retry button')
assert.match(indexTemplate, /v-else-if="!dailyVideos\.length"/, 'empty published video lists should have an explicit visitor message')
assert.match(indexTemplate, /v-else class="teacher-empty"/, 'an explicitly hidden teacher should render an empty state')
const teacherHeroImage = indexPage.match(/<image\b[^>]*class="mentor-portrait"[^>]*>/)?.[0] || ''
assert.match(teacherHeroImage, /:src="portrait"/, 'teacher hero should use its resolved portrait')
assert.match(teacherHeroImage, /aria-label=/, 'teacher portrait should have an accessible description')
assert.match(teacherHeroImage, /@error="portraitFailed = true"/, 'teacher hero should handle image failures')
assert.doesNotMatch(teacherHeroImage, /lazy-load/, 'the dominant teacher portrait should load eagerly')
for (const label of ['老师日常', '课程报名', '预约咨询', '认识自己']) {
  assert.ok(indexTemplate.includes(label), `home should expose the ${label} destination`)
}
const homeActions = indexTemplate.match(/<button\b[^>]*>/g) || []
assert.ok(homeActions.length >= 8, 'home destinations should use native interactive buttons')
for (const action of homeActions) assert.match(action, /@click=/, 'each home button should have a working action')
assert.match(indexPage, /button:focus-visible\s*\{[^}]*outline:/, 'native home controls should keep a visible keyboard focus state')
assert.match(indexPage, /safe-area-inset-top/, 'the custom home header should account for the top safe area')
assert.match(indexPage, /\.studio-home\s*\{[^}]*background:\s*#F7F5F0[^}]*color:\s*#282A27/i, 'home should use the warm paper and ink palette')
assert.doesNotMatch(indexTemplate, /AI 对话|问 AI/, 'home should remain focused on the teacher and growth content')

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
assert.match(bookingPage, /\.booking-page\s*\{[^}]*safe-area-inset-bottom/, 'booking layout should reserve the device bottom safe area')
assert.match(bookingPage, /userErrorMessage/, 'booking page should surface normalized request errors')
assert.match(bookingPage, /title:\s*userErrorMessage\(error,\s*'提交失败，填写内容已保留，请重试'\)/, 'booking submit should keep a fallback while showing specific API errors')
assert.match(bookingPage, /<button\b[^>]*class=["']primary-button["'][^>]*:loading=["']submitting["'][^>]*:disabled=["']submitting["'][^>]*@click=["']submit["']/, 'booking submit action should preserve its native button and duplicate-submit guard')
assert.match(bookingPage, /fieldErrors/, 'booking page should expose inline field validation errors')
assert.match(bookingPage, /v-if=["']fieldErrors\.contactName["']/, 'booking contact name should render an inline validation error')
assert.match(bookingPage, /v-if=["']fieldErrors\.phone["']/, 'booking phone should render an inline validation error')
assert.match(bookingPage, /:aria-invalid=["']!!fieldErrors\.contactName["']/, 'booking contact name input should expose aria-invalid when invalid')
assert.match(bookingPage, /:aria-invalid=["']!!fieldErrors\.phone["']/, 'booking phone input should expose aria-invalid when invalid')

const learnPage = readFileSync('src/pages/learn/learn.vue', 'utf8')
const learningPageStateSource = readFileSync('src/utils/learningPageState.js', 'utf8')

assert.match(learningPageStateSource, /normalizeTeachers/, 'learn state utility should normalize teacher profile data from site config')
assert.match(learningPageStateSource, /normalizeCoursewareItems/, 'learn state utility should normalize courseware and course data from site config')
assert.match(learningPageStateSource, /function\s+flattenLearningMaterials\(/, 'learn state should flatten course material types into category rows')
assert.match(learningPageStateSource, /function\s+resolveLearningCategory\(/, 'learn state should resolve one-time navigation intent without corrupting the active category')
for (const helper of ['createLearningCourseEntries', 'createLearningQuoteEntries', 'createLearningTagEntries', 'learningTabTransition']) {
  assert.match(learningPageStateSource, new RegExp(`function\\s+${helper}\\(`), `learn state should expose tested ${helper} behavior`)
}
assert.match(learnPage, /resolveContentAsset/, 'learn page should resolve backend teacher and course image assets safely')
const learnTemplate = learnPage.slice(learnPage.indexOf('<template>'), learnPage.lastIndexOf('</template>') + 11)
const learnTeacherSections = learnTemplate.match(/<section\b[^>]*class=["'][^"']*learn-teacher[^"']*["'][^>]*>/g) || []
assert.equal(learnTeacherSections.length, 1, 'learn page should render exactly one compact teacher introduction section')
assert.match(learnPage, /主讲老师/, 'learn page should introduce the teacher in concise Chinese copy')
assert.match(learnPage, /teacher\.name/, 'teacher profile should render the teacher name')
assert.match(learnPage, /teacher\.title/, 'teacher profile should render the teacher title')
assert.match(learnPage, /teacher\.bio/, 'teacher profile should render the teacher biography')
assert.match(learnPage, /teacherTagEntries/, 'teacher profile should render teacher expertise tags through stable entries')
const learnTeacherToggle = learnPage.match(/<button\b[^>]*class=["'][^"']*learn-teacher__toggle[^"']*["'][^>]*>/)?.[0] || ''
assert.match(learnTeacherToggle, /^<button/, 'compact teacher introduction should use a native accessible expand button')
assert.match(learnTeacherToggle, /@click=["']toggleTeacher["']/, 'native teacher introduction button should support pointer and keyboard activation')
assert.match(learnTeacherToggle, /:aria-expanded=["']teacherExpanded["']/, 'teacher introduction toggle should expose its expanded state')
assert.match(learnTeacherToggle, /aria-controls=["']learn-teacher-details["']/, 'teacher introduction toggle should identify the expandable details')

const learnTabList = learnPage.match(/<view\b[^>]*class=["'][^"']*learn-tabs[^"']*["'][^>]*>/)?.[0] || ''
assert.match(learnTabList, /role=["']tablist["']/, 'learning categories should expose tablist semantics')
const learnTabs = learnPage.match(/<button\b(?=[^>]*class=["'][^"']*learn-tab[^"']*["'])(?=[^>]*role=["']tab["'])[^>]*>/g) || []
assert.equal(learnTabs.length, 3, 'learning center should expose exactly three category tabs')
for (const tab of learnTabs) {
  assert.match(tab, /:aria-selected=/, 'each learning tab should expose its selected state')
  assert.match(tab, /:tabindex=/, 'each learning tab should participate in roving keyboard focus')
  assert.match(tab, /@click=/, 'each learning tab should support tap and click selection')
  assert.match(tab, /@keydown=/, 'each learning tab should support Enter, Space, and arrow keys')
  assert.match(tab, /aria-controls=["']learn-panel-(?:course|material|quote)["']/, 'each learning tab should identify its controlled panel')
  assert.equal((tab.match(/@keydown=/g) || []).length, 1, 'each learning tab should declare one keyboard event binding')
}
for (const category of ['老师日常', '学习资料', '课堂札记']) {
  assert.equal((learnTemplate.match(new RegExp(`>${category}<`, 'g')) || []).length, 1, `learning center should expose one ${category} tab`)
}
assert.match(learnPage, /onShow\(consumeNavigationIntent\)/, 'learning center should consume home navigation intent every time the tab is shown')
assert.match(learnPage, /readLearningNavIntent\(\)/, 'learning center should use the one-time read-and-clear navigation intent')
assert.match(learnPage, /function\s+selectCategory\(category\)\s*\{[\s\S]*?resolveLearningCategory\(activeCategory\.value,\s*category\)/, 'category selection should retain the current valid learning category when no intent is provided')
assert.match(learnPage, /function\s+consumeNavigationIntent\(\)\s*\{\s*selectCategory\(readLearningNavIntent\(\)\)\s*\}/, 'navigation intent should flow through the shared category visibility rules')

const learnTeacherImage = learnPage.match(/<image\b[^>]*class=["'][^"']*learn-teacher__image[^"']*["'][^>]*>/)?.[0] || ''
assert.match(learnTeacherImage, /:src=["']teacherImage["']/, 'teacher portrait should use a resolved render source')
assert.match(learnTeacherImage, /role=["']img["']/, 'teacher portrait should expose image semantics on H5')
assert.match(learnTeacherImage, /:aria-label=["']teacherImageLabel["']/, 'teacher portrait should expose a meaningful accessible label')
assert.match(learnTeacherImage, /@error=["']onTeacherImageError["']/, 'teacher portrait should provide a local fallback')
assert.doesNotMatch(learnTeacherImage, /lazy-load/, 'dominant teacher portrait should load eagerly')
assert.match(learnPage, /const\s+portrait\s*=\s*teacher\.value\?\.avatar\s*===\s*['"]\/static\/avatars\/9\.png['"]\s*\?\s*['"]["']\s*:\s*teacher\.value\?\.avatar/, 'learning center should reject the obsolete generic teacher avatar')
assert.match(learnPage, /resolveContentAsset\(portraitSource,\s*TEACHER_FALLBACK\)/, 'teacher portrait should resolve sanitized backend content before rendering')
assert.match(learnPage, /teacherImageFallbackUsed/, 'teacher portrait fallback should be applied only once')
assert.match(learnPage, /\.learn-teacher__portrait\s*\{[^}]*aspect-ratio:\s*4\s*\/\s*5/, 'teacher portrait should reserve an editorial 4:5 frame')

assert.match(learnPage, /class=["'][^"']*course-list[^"']*["']/, 'learn page should render a simple course media list')
assert.match(learnPage, /class=["'][^"']*course-row[^"']*["']/, 'learn page should render courses as lightweight media rows')
assert.match(learnPage, /courseEntry\.item\.materialTypes/, 'course rows should expose material types')
assert.match(learnPage, /courseEntry\.item\.duration/, 'course rows should expose duration metadata')
assert.match(learnPage, /courseEntry\.item\.description/, 'course rows should expose a short description')
assert.doesNotMatch(learnTemplate, /course\.bullets|bulletIndex|::bullet::/, 'course bullet data should not be rendered on the minimal learning page')
const learnCourseCovers = learnPage.match(/<image\b[^>]*class=["'][^"']*course-row__cover[^"']*["'][^>]*>/g) || []
assert.ok(learnCourseCovers.length >= 1, 'course rows should keep one compact cover visual')
for (const image of learnCourseCovers) {
  assert.match(image, /:aria-label=["']courseEntry\.item\.title["']/, 'course cover should expose its actual video title')
  assert.match(image, /@error=["']onCourseImageError\(/, 'course covers should provide a one-shot local fallback')
}
assert.match(learnPage, /function\s+learnCourseCover\(course,\s*courseKey\)/, 'learn page should map unsuitable legacy covers from stable course identity')
assert.ok(learnPage.includes('\\/static\\/wheel\\.png'), 'learn page should recognize the legacy wheel cover')
assert.match(learnPage, /resolveContentAsset\(learnCourseCover\(course,\s*courseKey\),\s*courseFallback\(courseKey\)\)/, 'course covers should resolve mapped content assets from stable identity')
assert.match(learnPage, /courseImageFallbackUsed/, 'course cover fallback should be applied only once per stable course key')
assert.match(learnPage, /courseImages\.value\[courseKey\]/, 'course image state should be keyed by stable course identity rather than array position')
assert.match(learnPage, /function\s+openPublishedCourse\(course\)\s*\{[\s\S]*?classroomContentRoute\(/, 'course rows should use the canonical classroom detail route')
assert.match(learnTemplate, /<button\b(?=[^>]*class=["'][^"']*course-row[^"']*["'])(?=[^>]*@click=["']openPublishedCourse\(courseEntry\.item\)["'])[^>]*>/, 'published course rows should open their classroom detail')
assert.match(learnTemplate, /<button\b(?=[^>]*class=["'][^"']*material-row[^"']*["'])(?=[^>]*@click=["']openPublishedMaterial\(material\)["'])[^>]*>/, 'published material rows should open their classroom detail')

assert.match(learnTemplate, /activeCategory\s*===\s*'course'[\s\S]*?class=["'][^"']*course-list/, 'course category should render the course list')
assert.match(learnTemplate, /activeCategory\s*===\s*'material'[\s\S]*?v-for=["']material in materialItems["']/, 'material category should render every flattened material row')
assert.match(learnTemplate, /activeCategory\s*===\s*'quote'[\s\S]*?v-for=["']quoteEntry in quoteEntries["']/, 'quote category should render all teacher quotes with stable entries')
assert.match(learnPage, /flattenLearningMaterials\(coursewareItems\.value\)/, 'material category should derive rows from normalized courses')
assert.match(learnPage, /课程正在准备中/, 'course category should provide a dedicated empty state')
assert.match(learnPage, /课件资料整理中/, 'material category should provide a dedicated empty state')
assert.match(learnPage, /老师语录整理中/, 'quote category should provide a dedicated empty state')
for (const category of ['course', 'material', 'quote']) {
  assert.match(learnTemplate, new RegExp(`id=["']learn-panel-${category}["'][^>]*role=["']tabpanel["'][^>]*aria-labelledby=["']learn-tab-${category}["']`), `${category} panel should be linked to its tab with tabpanel semantics`)
}
assert.match(learnPage, /class=["'][^"']*type-index[^"']*["']/, 'the type index should remain available late in the page')
assert.match(learnPage, /\.learn-teacher__bio\s*\{[^}]*font-size:\s*(?:2[6-9]|[3-9]\d|\d{3,})rpx/, 'teacher biography should remain readable at 26rpx or larger')
assert.match(learnPage, /\.course-row__desc\s*\{[^}]*font-size:\s*(?:2[6-9]|[3-9]\d|\d{3,})rpx/, 'course descriptions should remain readable at 26rpx or larger')
assert.match(learnPage, /\.material-row__type\s*\{[^}]*font-size:\s*(?:2[2-9]|[3-9]\d|\d{3,})rpx/, 'material metadata should remain readable at 22rpx or larger')
assert.match(learnPage, /\.course-row__duration\s*\{[^}]*font-size:\s*(?:2[2-9]|[3-9]\d|\d{3,})rpx/, 'duration metadata should remain readable at 22rpx or larger')
assert.match(learnPage, /\.type-index__item\s*\{[^}]*font-size:\s*(?:2[2-9]|[3-9]\d|\d{3,})rpx/, 'type index text should remain readable at 22rpx or larger')
assert.doesNotMatch(learnPage, /font-size:\s*19rpx/, 'learn page should not use 19rpx content text')
assert.match(learnTemplate, /<button\b(?=[^>]*class=["'][^"']*type-index__item[^"']*["'])(?=[^>]*@click=["']openTypeDetail\(type\.id\)["'])[^>]*>/, 'type index entries should open their matching type detail')
assert.doesNotMatch(learnPage, /class=["'][^"']*ios-card[^"']*["']/, 'learn editorial sections should not fall back to generic ios-card surfaces')
assert.doesNotMatch(learnPage, /background-image|@keyframes|animation\s*:|(?:backdrop-)?filter\s*:/, 'learn page should avoid heavy visual effects')
assert.match(learnPage, /\.learn-header\s*\{[^}]*padding:/, 'learn header should remain a compact editorial introduction')
assert.match(learnPage, /\.learn\s*\{[^}]*background:\s*var\(--nx-page-bg,\s*#F7F5F0\)[^}]*color:\s*var\(--nx-text,\s*#282A27\)/, 'learn page should use the warm shared page and text tokens')
assert.match(learnPage, /\.learn-teacher\s*\{[^}]*background:\s*var\(--nx-surface,\s*#FFFFFF\)/, 'learn teacher section should use a white reading surface')
assert.match(learnPage, /\.learn-tab--active::before\s*\{[^}]*background:\s*#A55C3B/, 'active learning tab should have a visible warm accent indicator')
assert.match(learnPage, /v-model="searchQuery"/, 'daily sharing should offer a working content search')
assert.match(learnPage, /:aria-pressed="activeTopic === topic"/, 'topic filters should expose their selected state')
assert.match(learnPage, /@click="clearSearch"/, 'empty filtered results should offer a search reset')

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
assert.match(learnPage, /:key=["']courseEntry\.key["']/, 'course rows should use stable utility keys')
assert.match(learnPage, /:key=["']tagEntry\.key["']/, 'teacher tags should use stable utility keys')
assert.match(learnPage, /:key=["']quoteEntry\.key["']/, 'quotes should use stable utility keys')
assert.match(learningPageStateSource, /::material::/, 'flattened material keys should include an explicit material identity delimiter')
assert.doesNotMatch(learnTemplate, /:key=["'][^"']*(?:index|Index)[^"']*["']/, 'learning rows should not use array indexes in Vue keys')

const learnRetry = learnPage.match(/<button\b[^>]*class=["'][^"']*learn-retry[^"']*["'][^>]*>/)?.[0] || ''
assert.match(learnRetry, /^<button/, 'learn retry should use native keyboard-accessible button semantics')
assert.match(learnRetry, /@click=["']loadContent["']/, 'learn retry should preserve mini-program and H5 activation')
assert.match(learnPage, /handleActionKeydown\(event,\s*\(\)\s*=>\s*activateAction\(action,\s*event\)\)/, 'learn keydown handler should delegate to the behavior-tested activation filter')
assert.match(learnPage, /learningTabTransition\(category,\s*event\?\.key\)/, 'tab keyboard handling should delegate to the tested transition utility')
const learnRoleButtons = learnPage.match(/<view\b(?=[^>]*role=["']button["'])[^>]*>/g) || []
for (const action of learnRoleButtons) {
  assert.equal((action.match(/@keydown=/g) || []).length, 1, 'each learn action should declare exactly one keydown binding')
}
assert.doesNotMatch(learnPage, /@keydown\./, 'learn keydown modifiers must not compile duplicate WXML attributes or intercept Tab')
assert.match(learnPage, /\.learn-retry\s*\{[^}]*min-height:\s*88rpx/, 'learn retry should keep an 88rpx touch target')
assert.match(learnPage, /\.learn-tab:focus-visible[\s\S]*outline:/, 'learning tabs should expose a visible keyboard focus state')
assert.match(learnPage, /\.learn-tab\s*\{[^}]*min-height:\s*(?:8[8-9]|9\d|[1-9]\d{2,})rpx/, 'learning category tabs should keep an 88rpx minimum touch target')
assert.match(learnPage, /\.learn-teacher__toggle\s*\{[^}]*min-height:\s*88rpx/, 'teacher details toggle should keep an 88rpx touch target')
const learnTabletMedia = learnPage.match(/@media\s+screen\s+and\s+\(min-width:\s*768px\)\s*\{([\s\S]*?)\n\}/)?.[1] || ''
assert.doesNotMatch(learnTabletMedia, /\.course-list\s*\{[^}]*grid-template-columns:\s*repeat\(2/, 'tablet courses should remain a readable single-column list')
assert.doesNotMatch(learnPage, /\.course-list\s*\{[^}]*grid-template-columns:\s*repeat\(2/, 'courses should remain a readable single-column list at every viewport width')
assert.doesNotMatch(learnPage, /min-width:\s*(?:7[5-9]\d|[89]\d{2}|\d{4,})rpx/, 'learning center should not force horizontal overflow at phone widths')

const profilePage = readFileSync('src/pages/profile/profile.vue', 'utf8')
const profileEditPage = readFileSync('src/pages/profile-edit/profile-edit.vue', 'utf8')
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

assert.match(profilePage, /await\s+ensureLogin\(\)/, 'profile page should use the shared WeChat login integration')
assert.match(profilePage, /uni\.navigateTo\(\{\s*url:\s*['"]\/pages\/profile-edit\/profile-edit['"]\s*\}\)/, 'profile page should open the dedicated profile editor')
assert.match(profileEditPage, /open-type="chooseAvatar"/, 'profile editor should keep the WeChat avatar slot')
assert.match(profileEditPage, /type="nickname"/, 'profile editor should keep the WeChat nickname slot')
assert.doesNotMatch(`${profilePage}\n${profileEditPage}`, /open-type="getPhoneNumber"/, '未接通后端前，手机号授权入口不能对用户露出')
assert.doesNotMatch(`${profilePage}\n${profileEditPage}`, /@getphonenumber="onGetPhoneNumber"/, '未接通后端前，不应绑定可见手机号授权占位事件')
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
assert.match(relationPage, /\.type-chip\s*\{[\s\S]*min-height:\s*(?:8[8-9]|9\d|[1-9]\d{2,})rpx/, 'relation type chips should keep at least an 88rpx touch target')
assert.match(relationPage, /<button\b[\s\S]*v-for=["']t in allTypes["'][\s\S]*class=["']type-chip nx-focusable["'][\s\S]*:aria-label=/, 'relation type chips should use button semantics with accessibility labels')
assert.match(relationPage, /hover-class=["']type-chip--pressed["']/, 'relation type chips should expose a hover/press visual state')
assert.match(relationPage, /\.type-chip--pressed\s*\{[\s\S]*(?:opacity|transform)/, 'relation chip pressed state should have visible feedback')

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

await import('../src/pages/index/index.test.mjs')
console.log('ui compatibility tests passed')
