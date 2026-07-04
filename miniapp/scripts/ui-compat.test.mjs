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

const chatPage = readFileSync('src/pages/chat/chat.vue', 'utf8')
assert.match(
  chatPage,
  /var\(--window-bottom,\s*0px\)/,
  'chat page must provide a fallback for --window-bottom',
)
assert.match(chatPage, /100dvh/, 'chat page should use dynamic viewport units on H5/iOS')
assert.match(chatPage, /\.chat__clear\s*\{[\s\S]*min-height:\s*88rpx/, 'chat clear button should keep an 88rpx touch target')
assert.match(chatPage, /\.suggestion\s*\{[\s\S]*min-height:\s*88rpx/, 'chat suggestion chips should keep an 88rpx touch target')
assert.match(chatPage, /\.composer__send\s*\{[\s\S]*min-height:\s*88rpx/, 'chat send button should keep an 88rpx touch target')

assert.match(chatPage, /retryMessage\(msg\)/, 'chat page should retry failed AI messages with their original question')
assert.match(chatPage, /copyAnswer\(msg\)/, 'chat page should expose answer copy action')
assert.match(chatPage, /copyText\(msg\.content\)/, 'chat answer copy should use tested clipboard helper')
assert.match(chatPage, /msg__action[\s\S]*:disabled="sending/, 'chat message actions should be disabled while sending')
assert.match(chatPage, /\.msg__action\s*\{[\s\S]*min-height:\s*88rpx/, 'chat message action buttons should keep an 88rpx touch target')

const h5Index = readFileSync('index.html', 'utf8')
assert.match(h5Index, /viewport-fit=cover/, 'H5 viewport meta should enable iOS safe-area env variables')

const appVue = readFileSync('src/App.vue', 'utf8')
assert.match(appVue, /\.page-stack\s*\{[\s\S]*safe-area-inset-bottom/, 'page-stack should reserve bottom safe area globally')
assert.match(appVue, /\.page-stack\s*\{[\s\S]*var\(--window-bottom,\s*0px\)/, 'page-stack should reserve H5 tabbar/window bottom globally')

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

const learnPage = readFileSync('src/pages/learn/learn.vue', 'utf8')
assert.match(learnPage, /loadError/, 'learn page should expose a non-blocking failure state')
assert.match(learnPage, /v-if="loading"/, 'learn page should render loading placeholders instead of a blank area')
assert.match(learnPage, /@click="loadContent"/, 'learn page should provide retry when site config fails')

const profilePage = readFileSync('src/pages/profile/profile.vue', 'utf8')
assert.match(profilePage, /profileLoading/, 'profile page should expose a loading state for non-blocking history fetch')
assert.match(profilePage, /v-if="profileLoading"/, 'profile page should render loading placeholder before empty states')
assert.match(profilePage, /loadTicket/, 'profile page should ignore stale concurrent loads')

const testPage = readFileSync('src/pages/test/test.vue', 'utf8')
assert.match(testPage, /answerLocked/, 'test page should guard rapid repeated option taps')
assert.match(testPage, /clearAdvanceTimer/, 'test page should clear pending navigation timers')
assert.match(testPage, /onUnload/, 'test page should cleanup timers on unload')

assert.match(learnPage, /getStoredSiteConfig/, 'learn page should render stored site config before network refresh')
assert.match(learnPage, /refreshSiteConfig/, 'learn page should refresh site config in the background')
assert.match(learnPage, /silent/, 'learn background refresh should avoid replacing cached content with a blocking state')

assert.match(profilePage, /wechatLoginReady/, 'profile page should expose a WeChat login integration slot')
assert.match(profilePage, /open-type="chooseAvatar"/, 'profile page should keep WeChat avatar slot')
assert.match(profilePage, /type="nickname"/, 'profile page should keep WeChat nickname slot')
assert.match(profilePage, /open-type="getPhoneNumber"/, 'profile page should expose WeChat getPhoneNumber authorization')
assert.match(profilePage, /@getphonenumber="onGetPhoneNumber"/, 'profile page should handle WeChat phone authorization result')
assert.match(profilePage, /后端暂未开通/, 'phone authorization should degrade with a backend-placeholder toast')


const resultPage = readFileSync('src/pages/result/result.vue', 'utf8')
assert.match(resultPage, /v-else-if="reportError"/, 'result page should render report failure state before falling back to manual fetch')
assert.match(resultPage, /report__retry[\s\S]*@click="loadReportContent"/, 'result page should allow retrying report content fetch from the error state')
assert.match(resultPage, /userErrorMessage/, 'result page should surface normalized request errors')
assert.match(resultPage, /normalizeLastResult/, 'result page should validate cached result schema before rendering')
assert.match(resultPage, /测试结果已失效/, 'result page should give feedback when cached result schema is invalid')

const relationPage = readFileSync('src/pages/relation/relation.vue', 'utf8')
assert.match(relationPage, /isValidTypeId/, 'relation page should validate incoming and selected type ids')
assert.match(relationPage, /型号参数无效/, 'relation page should explain invalid query type before navigation')
assert.match(relationPage, /\/pages\/test\/test/, 'relation page should return to the test page for invalid query type')
assert.match(relationPage, /\.chip\s*\{[\s\S]*min-height:\s*88rpx/, 'relation type chips should keep an 88rpx touch target')

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
