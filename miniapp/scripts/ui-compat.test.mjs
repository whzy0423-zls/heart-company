import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

const packageJson = JSON.parse(readFileSync("package.json", "utf8"));
const learnClassroomSource = readFileSync("src/pages/learn/learn.vue", "utf8");
const classroomPage = statSync("src/pages/classroom/classroom.vue", { throwIfNoEntry: false })
  ? readFileSync("src/pages/classroom/classroom.vue", "utf8")
  : "";
const classroomDetailPage = statSync("src/pages/classroom-detail/classroom-detail.vue", {
  throwIfNoEntry: false,
})
  ? readFileSync("src/pages/classroom-detail/classroom-detail.vue", "utf8")
  : "";

assert.match(
  packageJson.scripts["test:config"],
  /src\/utils\/classroom-progress-order\.test\.mjs/,
  "default config tests should cover classroom progress and purchase states",
);

assert.match(
  learnClassroomSource,
  /class="classroom-entry/,
  "learning page should expose the teacher classroom section",
);
assert.match(
  learnClassroomSource,
  /loadClassroomPreview/,
  "classroom preview should load independently from cached learning content",
);
assert.match(
  learnClassroomSource,
  /retryClassroomPreview/,
  "classroom preview failure should expose a separate retry",
);
assert.match(
  learnClassroomSource,
  /coursewareItems/,
  "classroom preview must preserve legacy home course fallback",
);
assert.match(
  classroomPage,
  /min-height:\s*88rpx/,
  "classroom actions should keep accessible touch targets",
);
assert.match(
  classroomDetailPage,
  /min-height:\s*88rpx/,
  "classroom detail actions should keep accessible touch targets",
);
assert.match(
  classroomPage,
  /role="progressbar"[\s\S]*aria-valuenow/,
  "continue-learning should expose accessible progress semantics",
);
assert.match(
  classroomDetailPage,
  /payment-panel[\s\S]*aria-live="polite"/,
  "payment state changes should be announced",
);

assert.equal(packageJson.scripts["dev:h5"], "uni -p h5");
assert.equal(packageJson.scripts["build:h5"], "uni build -p h5");
assert.equal(
  packageJson.dependencies["@dcloudio/uni-h5"],
  packageJson.dependencies["@dcloudio/uni-app"],
);
for (const requiredTest of [
  "src/pages/profile/profile.session.test.mjs",
  "src/pages/booking-records/booking-records.session.test.mjs",
  "src/utils/bookingDisplay.test.mjs",
  "src/utils/bookingSession.test.mjs",
  "src/pages/learn/learn.content-state.test.mjs",
  "src/pages/classroom/classroom.test.mjs",
  "src/pages/classroom-detail/classroom-detail.test.mjs",
  "src/pages/result/result.recommendation.test.mjs",
  "src/pages/index/index.test.mjs",
]) {
  assert.match(
    packageJson.scripts["test:config"],
    new RegExp(requiredTest.replaceAll(".", "\\.")),
    `test:config should run ${requiredTest}`,
  );
}
assert.match(
  packageJson.scripts["test:config"],
  /node src\/pages\/booking-detail\/booking-detail\.session\.test\.mjs/,
  "the default test command should cover appointment detail auth and stale-response behavior",
);

const pagesConfig = readFileSync("src/pages.json", "utf8");
const pagesConfigJson = JSON.parse(pagesConfig);
function configuredPage(path) {
  const mainPage = pagesConfigJson.pages.find((page) => page.path === path);
  if (mainPage) return mainPage;
  for (const subpackage of pagesConfigJson.subPackages || []) {
    const page = subpackage.pages.find(
      (item) => `${subpackage.root}/${item.path}`.replace(/\/+/g, "/") === path,
    );
    if (page) return page;
  }
  return undefined;
}
assert.doesNotMatch(
  pagesConfig,
  /pages\/chat\/chat/,
  "pages.json must not register the removed chat page",
);
assert.doesNotMatch(pagesConfig, /问 AI|AI 对话/, "tabBar must not expose an AI chat entry");
assert.equal(
  statSync("src/pages/chat", { throwIfNoEntry: false }),
  undefined,
  "removed chat page directory should stay deleted",
);
assert.equal(
  configuredPage("pages/profile-edit/profile-edit")?.style?.navigationBarTitleText,
  "个人资料",
);
assert.equal(
  configuredPage("pages/booking-records/booking-records")?.style?.navigationBarTitleText,
  "预约记录",
);
assert.equal(
  configuredPage("pages/booking-detail/booking-detail")?.style?.navigationBarTitleText,
  "预约详情",
);

const h5Index = readFileSync("index.html", "utf8");
assert.match(
  h5Index,
  /viewport-fit=cover/,
  "H5 viewport meta should enable iOS safe-area env variables",
);

const appVue = readFileSync("src/App.vue", "utf8");

const appleMobileStyle = readFileSync("src/styles/apple-mobile.css", "utf8");
assert.match(
  appVue,
  /@import ['"]\.\/styles\/apple-mobile\.css['"];/,
  "App.vue should import shared Apple/iOS mobile tokens",
);
for (const token of ["--nx-bg", "--nx-primary", "--nx-card", "--nx-radius", "--nx-safe-bottom"]) {
  assert.match(appleMobileStyle, new RegExp(token), `apple-mobile.css should define ${token}`);
}
for (const token of [
  "--nx-brand-900",
  "--nx-brand-700",
  "--nx-accent-gold",
  "--nx-page-bg",
  "--nx-surface",
  "--nx-surface-soft",
  "--nx-text",
  "--nx-text-muted",
  "--nx-border",
  "--nx-danger",
  "--nx-success",
]) {
  assert.match(
    appleMobileStyle,
    new RegExp(`${token}\\s*:`),
    `apple-mobile.css should define ${token}`,
  );
}
for (const className of [
  ".ios-page",
  ".ios-card",
  ".ios-button",
  ".ios-section",
  ".ios-safe-bottom",
]) {
  assert.match(
    appleMobileStyle,
    new RegExp(className.replace(".", "\\.") + "\\s*\\{"),
    `apple-mobile.css should define ${className}`,
  );
}
for (const className of [
  ".nx-page-hero",
  ".nx-section-head",
  ".nx-panel",
  ".nx-state",
  ".nx-tag",
  ".nx-focusable",
]) {
  assert.match(
    appleMobileStyle,
    new RegExp(className.replace(".", "\\.") + "\\s*\\{"),
    `apple-mobile.css should define ${className}`,
  );
}
assert.match(
  appleMobileStyle,
  /\.nx-focusable:focus\s*\{[^}]*outline\s*:\s*4rpx\s+solid\s+var\(--nx-brand-700\)\s*;/,
  ".nx-focusable:focus should expose a brand-token focus outline",
);
assert.match(
  appleMobileStyle,
  /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{\s*\.nx-focusable\s*\{[^}]*animation\s*:\s*none\s*;/,
  "reduced-motion styles should disable .nx-focusable animation",
);
assert.match(
  appleMobileStyle,
  /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{\s*\.nx-focusable\s*\{[^}]*transition\s*:\s*none\s*;/,
  "reduced-motion styles should disable .nx-focusable transition",
);
assert.match(
  appleMobileStyle,
  /min-height:\s*88rpx/,
  "Apple/iOS buttons should keep an 88rpx touch target",
);
assert.match(
  appleMobileStyle,
  /safe-area-inset-bottom/,
  "Apple/iOS style tokens should reserve safe-area bottom",
);

function assertRootViewClasses(source, file, classNames) {
  const match = source.match(/<template>\s*<view\s+class=["']([^"']+)["']/);
  assert.ok(match, `${file} should render a root view with static classes`);
  const actual = match[1].split(/\s+/);
  for (const className of classNames) {
    assert.ok(actual.includes(className), `${file} root should include ${className}`);
  }
}

for (const file of [
  "src/pages/index/index.vue",
  "src/pages/result/result.vue",
  "src/pages/profile/profile.vue",
]) {
  const source = readFileSync(file, "utf8");
  assert.match(source, /ios-page/, `${file} should opt into shared Apple/iOS page styling`);
  assert.match(
    source,
    /(?:ios-card|nx-card)/,
    `${file} should opt into a shared design-system card surface`,
  );
}

for (const file of [
  "src/pages/relation/relation.vue",
  "src/pages/test/test.vue",
  "src/pages/learn/learn.vue",
  "src/pages/booking/booking.vue",
]) {
  const source = readFileSync(file, "utf8");
  assertRootViewClasses(source, file, ["page-stack", "ios-page", "ios-safe-bottom"]);
}

const indexPage = readFileSync("src/pages/index/index.vue", "utf8");
const homeTemplate = stripMarkupAndCssComments(
  topLevelVueSection(indexPage, "template") || "",
);
const homeStyle = vueSection(indexPage, "style") || "";

function staticClassTokens(tag) {
  const match = tag.match(/\sclass=["']([^"']*)["']/);
  return match ? match[1].trim().split(/\s+/).filter(Boolean) : [];
}

function assertKeyboardViewControl(tag, description, handler) {
  assert.match(tag, /\srole=["']button["']/, `${description} should use web button semantics`);
  assert.match(
    tag,
    /\saria-role=["']button["']/,
    `${description} should expose miniapp button semantics`,
  );
  assert.match(
    tag,
    /\stabindex=["']0["']/,
    `${description} should participate in keyboard focus order`,
  );
  assert.match(
    tag,
    new RegExp(`\\s@keydown\\.enter=["']${handler}["']`),
    `${description} should activate with Enter`,
  );
  assert.match(
    tag,
    new RegExp(`\\s@keydown\\.space\\.prevent=["']${handler}["']`),
    `${description} should activate with Space without scrolling`,
  );
}

function assertTemplateOrder(source, selectors, description) {
  let previous = -1;
  for (const selector of selectors) {
    const current = source.indexOf(selector);
    assert.ok(current >= 0, `${description} should render ${selector}`);
    assert.ok(current > previous, `${description} should keep ${selector} in semantic order`);
    previous = current;
  }
}

function cssDeclarationBlocksForSelector(source, selector) {
  return [...source.matchAll(/([^{}]+)\{([^{}]*)\}/g)]
    .filter(([, selectors]) => selectors.split(",").some((item) => item.trim() === selector))
    .map(([, , declarations]) => declarations);
}

function cssDeclarationsForSelector(source, selector) {
  return cssDeclarationBlocksForSelector(source, selector).join("\n");
}

assertTemplateOrder(
  homeTemplate,
  [
    'class="expert-hero nx-card"',
    'class="enterprise-services"',
    'class="test-game nx-card"',
    'class="classroom-preview"',
    'class="secondary-entries"',
    'class="enterprise-final-cta"',
  ],
  "personal-expert home",
);
assert.doesNotMatch(
  homeTemplate,
  /class=["']home-nav(?:__[^"'\s]*)?["']/,
  "personal-expert home should not render the profile-only brand strip",
);
assert.match(
  indexPage,
  /normalizePersonalExpertHome/,
  "home should render the normalized personal-expert content model",
);
assert.match(
  indexPage,
  /MINIAPP_HOME_ENTRY_BEHAVIORS/,
  "home secondary entries should keep fixed navigation metadata",
);
assert.doesNotMatch(
  indexPage,
  /entry\.(?:url|path|href|route)/,
  "configured home entries must not provide arbitrary navigation targets",
);
assert.match(
  indexPage,
  /getStoredSiteConfig/,
  "home should render stored configuration before refreshing",
);
assert.match(
  indexPage,
  /refreshSiteConfig/,
  "home should refresh stored configuration in the background",
);
assert.match(
  homeTemplate,
  /<NxAsyncState\b(?=[^>]*v-if=["']siteStale["'])(?=[^>]*state=["']stale["'])[^>]*>/,
  "home should keep stale cached content visible with a retry state",
);

const homeCarousel = homeTemplate.match(/<view\s+v-if=["']carousel\.items\.length["']\s+class=["']carousel["'][\s\S]*?<\/view>/)?.[0] || "";
assert.match(homeCarousel, /<swiper\b/, "home should render configured carousel content");
assert.match(
  homeCarousel,
  /:autoplay=["'][^"']*carousel\.items\.length\s*>\s*1[^"']*!carouselPaused[^"']*["']/,
  "home carousel should autoplay only multiple unpaused slides",
);
assert.match(homeCarousel, /:interval=["']carousel\.interval["']/, "home carousel should use configured timing");
assert.match(
  homeCarousel,
  /<image\b(?=[^>]*lazy-load)(?=[^>]*@error=["']removeCarouselItem\(item\.image\)["'])[^>]*>/,
  "home carousel should lazy-load images and remove only failed slides",
);
assert.match(
  homeCarousel,
  /<button\b[\s\S]*?class=["']carousel__toggle["'][\s\S]*?@click=["']toggleCarouselPaused["'][\s\S]*?>/,
  "home carousel should expose a pause or resume control",
);
assert.match(
  indexPage,
  /failedCarouselImages\s*=\s*new Set\(\)[\s\S]*failedCarouselImages\.add\(image\)/,
  "home should retain failed carousel image URLs across refreshes",
);

assert.match(
  homeTemplate,
  /<image\b(?=[^>]*class=["']expert-hero__image["'])(?=[^>]*@error=["']markTeacherImageError["'])[^>]*\/>[\s\S]*?<view\s+v-else\s+class=["']expert-hero__monogram["']/,
  "expert portrait should expose an image-error fallback",
);
assert.match(
  homeTemplate,
  /<NxAsyncState\s+v-if=["']classroomState === 'loading'["']\s+state=["']loading["']\s*\/>/,
  "home classroom preview should expose loading state",
);
assert.match(
  homeTemplate,
  /<NxAsyncState\b(?=[^>]*v-else-if=["']classroomState === 'error'["'])(?=[^>]*state=["']error["'])[^>]*>/,
  "home classroom preview should expose an independent retryable error state",
);
assert.match(
  homeTemplate,
  /<NxAsyncState\b(?=[^>]*v-else-if=["']classroomState === 'empty'["'])(?=[^>]*state=["']empty["'])[^>]*>/,
  "home classroom preview should expose a visitor-facing empty state",
);
assert.match(
  homeTemplate,
  /<image\b(?=[^>]*class=["']classroom-card__cover["'])(?=[^>]*lazy-load)(?=[^>]*@error=["']markCourseCoverError\(courseCoverKey\(item\)\)["'])[^>]*\/>[\s\S]*?<view\s+v-else\s+class=["']classroom-card__cover-fallback["']/,
  "classroom preview covers should lazy-load and fall back after image errors",
);

for (const className of [
  "expert-hero__secondary",
  "carousel__toggle",
  "enterprise-service",
  "test-game__cta",
  "classroom-card",
  "secondary-entry",
  "enterprise-final-cta__button",
]) {
  const touchBlocks = cssDeclarationBlocksForSelector(homeStyle, `.${className}`);
  assert.ok(touchBlocks.length > 0, `.${className} should define a CSS rule`);
  const minHeights = touchBlocks.flatMap((declarations) =>
    [...declarations.matchAll(/min-height:\s*(\d+)rpx\s*;/g)].map((match) => Number(match[1])),
  );
  assert.ok(
    minHeights.length > 0,
    `.${className} should define an explicit minimum touch height`,
  );
  assert.ok(
    minHeights.every((height) => height >= 88),
    `.${className} should keep every minimum touch height at or above 88rpx; got ${minHeights.join(", ")}`,
  );
}
assert.match(
  homeStyle,
  /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{[\s\S]*?\.home button\s*\{[^}]*transition:\s*none\s*;/,
  "home should disable button transitions for reduced motion",
);
for (const token of [
  "--nx-brand-900",
  "--nx-brand-700",
  "--nx-accent-gold",
  "--nx-page-bg",
  "--nx-surface",
  "--nx-text",
  "--nx-text-muted",
  "--nx-border",
]) {
  assert.match(indexPage, new RegExp(`var\\(${token}\\)`), `home should consume ${token}`);
}
assert.doesNotMatch(
  indexPage,
  /openChatPage|goChat|问 AI|AI 对话|打开 AI 对话/,
  "home must not expose the removed AI chat entry",
);

assert.match(
  appleMobileStyle,
  /--nx-page-bottom:\s*calc\([^;]*safe-area-inset-bottom[^;]*\)\s*;/,
  "the shared page-bottom token should reserve the device safe area",
);
assert.match(
  appleMobileStyle,
  /--nx-page-bottom:\s*calc\([^;]*var\(--window-bottom,\s*0px\)[^;]*\)\s*;/,
  "the shared page-bottom token should reserve H5 tabbar/window bottom",
);
assert.match(
  appleMobileStyle,
  /\.ios-safe-bottom\s*\{[^}]*padding-bottom:\s*var\(--nx-page-bottom\)\s*;/,
  "safe-bottom pages should consume the shared page-bottom token",
);

function collectVueFiles(dir) {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    return statSync(path).isDirectory()
      ? collectVueFiles(path)
      : path.endsWith(".vue")
        ? [path]
        : [];
  });
}

const pageVueTemplates = collectVueFiles("src/pages").map((file) => {
  const source = readFileSync(file, "utf8");
  return {
    file,
    template: stripMarkupAndCssComments(topLevelVueSection(source, "template") || ""),
  };
});
assert.match(
  pageVueTemplates.find(({ file }) => file.endsWith("/result/result.vue"))?.template || "",
  /@click=["']unlockReport["']/,
  "global page scans should reach controls after internal template branches",
);

for (const { file, template } of pageVueTemplates) {
  const buttons = openingTagsFor(template, "button");
  for (const button of buttons) {
    if (!button.includes(":loading=")) continue;
    assert.match(
      button,
      /\s(?::disabled|disabled)(?:=|\s|>)/,
      `${file} has a loading button without disabled state: ${button}`,
    );
  }
  const images = openingTagsFor(template, "image");
  for (const image of images) {
    if (image.includes("poster-img")) continue;
    if (/\slazy-load(?:=|\s|>|$)/.test(image)) continue;
    assert.match(
      image,
      /class=["'][^"']*(?:hero|avatar|portrait)[^"']*["']/,
      `${file} should reserve eager loading for above-the-fold identity imagery: ${image}`,
    );
    assert.match(
      image,
      /\s@error=/,
      `${file} eager identity image should expose an error fallback: ${image}`,
    );
  }
}

const bookingPage = readFileSync("src/pages/booking/booking.vue", "utf8");
const bookingTemplate = stripMarkupAndCssComments(vueSection(bookingPage, "template") || "");
const bookingStyle = stripMarkupAndCssComments(vueSection(bookingPage, "style") || "");

assert.match(bookingPage, /userErrorMessage/, "booking should surface normalized request errors");
assert.match(bookingPage, /fieldErrors/, "booking should expose inline field validation errors");
for (const field of ["contactName", "phone"]) {
  assert.match(
    bookingTemplate,
    new RegExp(`:aria-invalid=["']!!fieldErrors\\.${field}["']`),
    `booking ${field} should expose aria-invalid`,
  );
  assert.match(
    bookingTemplate,
    new RegExp(`v-if=["']fieldErrors\\.${field}["'][^>]*role=["']alert["']`),
    `booking ${field} should render a nearby live error`,
  );
}

assert.match(bookingPage, /const DRAFT_SAVE_DELAY = 250/, "booking should debounce draft writes");
assert.ok(
  bookingPage.indexOf("const draft = loadBookingDraft()") < bookingPage.indexOf("watch("),
  "booking should restore its draft before enabling autosave",
);
assert.match(
  bookingPage,
  /const restoredKindIndex = kindIndexFor\(draft\.kind\)[\s\S]*if \(restoredKindIndex >= 0\) kindIndex\.value = restoredKindIndex/,
  "booking should ignore unknown restored kinds and keep the enterprise default",
);
const bookingWatch =
  bookingPage.match(/watch\(\s*\[kindIndex, form\],([\s\S]*?)\{ deep: true \},\s*\)/)?.[1] || "";
assert.match(bookingWatch, /scheduleDraftSave/, "booking watch should schedule draft persistence");
assert.doesNotMatch(bookingWatch, /saveBookingDraft/, "booking watch should not write on every keystroke");
const scheduleDraftBody =
  sourceBracedBody(bookingPage, /function\s+scheduleDraftSave\s*\(\s*\)\s*\{/.exec(bookingPage)) || "";
assert.match(
  scheduleDraftBody,
  /clearTimeout\(draftSaveTimer\)[\s\S]*setTimeout\([\s\S]*DRAFT_SAVE_DELAY/,
  "booking draft scheduling should reset the prior timer and use the debounce delay",
);
const flushDraftBody =
  sourceBracedBody(bookingPage, /function\s+flushDraftSave\s*\(\s*\)\s*\{/.exec(bookingPage)) || "";
assert.match(
  flushDraftBody,
  /clearTimeout\(draftSaveTimer\)[\s\S]*saveBookingDraft\(currentDraft\(\)\)/,
  "booking lifecycle flush should synchronously preserve the latest draft",
);
assert.match(bookingPage, /onHide\(flushDraftSave\)/, "booking should flush drafts when hidden");
assert.match(bookingPage, /onUnload\(flushDraftSave\)/, "booking should flush drafts before unload");

assert.match(bookingPage, /onShow\(applyBookingIntent\)/, "booking should consume navigation intent on show");
assert.match(
  bookingPage,
  /const intent = consumeBookingIntent\(\)[\s\S]*const nextKindIndex = kindIndexFor\(intent\.kind\)/,
  "booking should consume one-time enterprise intent through the bounded kind map",
);
const bookingSubmitBody =
  sourceBracedBody(bookingPage, /async\s+function\s+submit\s*\(\s*\)\s*\{/.exec(bookingPage)) || "";
assert.match(
  bookingSubmitBody,
  /if \(submitting\.value\) return[\s\S]*submitting\.value = true/,
  "booking submit should remain single-flight",
);
assert.match(
  bookingSubmitBody,
  /await createBookingApi\(currentDraft\(\)\)[\s\S]*cancelPendingDraftSave\(\)[\s\S]*clearBookingDraft\(\)/,
  "successful booking should cancel delayed persistence before clearing its draft",
);
const resetFormBody =
  sourceBracedBody(bookingPage, /function\s+resetForm\s*\(\s*\)\s*\{/.exec(bookingPage)) || "";
assert.match(
  resetFormBody,
  /^\s*cancelPendingDraftSave\(\)[\s\S]*kindIndex\.value\s*=\s*ENTERPRISE_KIND_INDEX[\s\S]*selectedServiceModeIndex\.value\s*=\s*-1[\s\S]*form\.value\s*=\s*emptyForm\(\)[\s\S]*fieldErrors\.value\s*=\s*\{\s*contactName:\s*['"]["'],\s*phone:\s*['"]["']\s*\}[\s\S]*restoredDraftNotice\.value\s*=\s*false\s*$/,
  "booking form reset should cancel pending persistence and clear kind, service, fields, errors, and recovery notice",
);
const clearRestoredDraftBody =
  sourceBracedBody(bookingPage, /function\s+clearRestoredDraft\s*\(\s*\)\s*\{/.exec(bookingPage)) || "";
assert.match(
  clearRestoredDraftBody,
  /^\s*if \(submitting\.value\) return\s*clearBookingDraft\(\)\s*resetForm\(\)\s*$/,
  "restored draft clearing should stay inert during submit, clear storage, and reset the form",
);

assertTemplateOrder(
  bookingTemplate,
  [
    'class="enterprise-hero nx-card"',
    'class="enterprise-scenarios"',
    'class="enterprise-modes"',
    'class="enterprise-process"',
    'class="booking-form nx-card"',
  ],
  "enterprise booking flow",
);
assert.match(
  bookingTemplate,
  /<view\s+v-if=["']submitted["']\s+class=["']booking-success nx-card["']\s+aria-live=["']polite["']>/,
  "booking should announce its success state",
);
for (const handler of ["viewBookingRecords", "continueClassroom", "submitAnother"]) {
  assert.match(
    bookingTemplate,
    new RegExp(`<button[^>]*@click=["']${handler}["']`),
    `booking success should expose ${handler}`,
  );
}
assert.match(
  bookingTemplate,
  /v-if=["']restoredDraftNotice["'][^>]*aria-live=["']polite["']/,
  "restored drafts should be announced",
);
assert.match(
  bookingTemplate,
  /<button\s+class=["']draft-restored__clear["'][^>]*:disabled=["']submitting["'][^>]*@click=["']clearRestoredDraft["']/,
  "draft clearing should use a disabled native action while submitting",
);

for (const { model, label } of [
  { model: "form.contactName", label: "称呼" },
  { model: "form.phone", label: "手机号" },
  { model: "form.intent", label: "意向方向" },
  { model: "form.preferredTime", label: "期望时间" },
]) {
  assert.match(
    bookingTemplate,
    new RegExp(`<input\\b(?=[^>]*v-model=["']${model.replace(".", "\\.")}["'])(?=[^>]*aria-label=["']${label}["'])[^>]*>`),
    `booking ${model} should expose its visible label to assistive technology`,
  );
}
assert.match(
  bookingTemplate,
  /<textarea\b(?=[^>]*v-model=["']form\.message["'])(?=[^>]*aria-label=["']留言["'])[^>]*>/,
  "booking message should expose its visible label",
);

const h5BookingBlock =
  bookingPage.match(/<!--\s*#ifdef H5\s*-->([\s\S]*?)<!--\s*#endif\s*-->/)?.[1] || "";
assert.match(h5BookingBlock, /<button[^>]*disabled[^>]*>请在微信小程序内提交预约<\/button>/, "H5 should keep submit disabled");
assert.doesNotMatch(h5BookingBlock, /@click=["']submit["']/, "H5 booking should not bind submit");
const nonH5BookingBlock =
  bookingPage.match(/<!--\s*#ifndef H5\s*-->([\s\S]*?)<!--\s*#endif\s*-->/)?.[1] || "";
assert.match(
  nonH5BookingBlock,
  /<button\b(?=[^>]*class=["']booking-submit ios-button["'])(?=[^>]*:loading=["']submitting["'])(?=[^>]*:disabled=["']submitting["'])(?=[^>]*@click=["']submit["'])[^>]*>/,
  "miniapp booking should preserve loading, disabled, and submit behavior",
);

for (const selector of [
  ".enterprise-mode",
  ".draft-restored__clear",
  ".input",
  ".picker",
  ".booking-submit",
]) {
  assert.match(
    cssDeclarationsForSelector(bookingStyle, selector),
    /min-height:\s*(?:8[8-9]|9\d|[1-9]\d{2,})rpx\s*;/,
    `${selector} should keep at least an 88rpx touch target`,
  );
}
for (const token of [
  "--nx-brand-900",
  "--nx-brand-700",
  "--nx-accent-gold",
  "--nx-surface",
  "--nx-surface-soft",
  "--nx-text",
  "--nx-text-muted",
  "--nx-border",
  "--nx-danger",
]) {
  assert.match(bookingStyle, new RegExp(`var\\(${token}\\)`), `booking should consume ${token}`);
}
assert.match(
  bookingStyle,
  /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{[\s\S]*?transition:\s*none\s*;/,
  "booking should disable interaction transitions for reduced motion",
);

const learnPage = readFileSync("src/pages/learn/learn.vue", "utf8");
const learnTemplate = stripMarkupAndCssComments(vueSection(learnPage, "template") || "");
const learnStyle = stripMarkupAndCssComments(vueSection(learnPage, "style") || "");

assert.match(learnPage, /normalizeTeachers/, "learn should normalize teacher profiles");
assert.match(learnPage, /normalizeCoursewareItems/, "learn should normalize course directions");
assert.match(learnPage, /getStoredSiteConfig/, "learn should render cached content first");
assert.match(learnPage, /refreshSiteConfig/, "learn should refresh cached content in the background");
assert.match(
  learnPage,
  /const ticket = \+\+loadTicket[\s\S]*if \(ticket !== loadTicket\) return/,
  "learn content refresh should reject late callbacks",
);
assert.match(
  learnPage,
  /const ticket = \+\+classroomTicket[\s\S]*if \(ticket !== classroomTicket\) return/,
  "learn classroom preview should reject late callbacks",
);
assert.match(
  learnPage,
  /Promise\.allSettled\(\[[\s\S]*listClassroomSeriesApi[\s\S]*listClassroomStandaloneApi/,
  "learn classroom preview should preserve partial results when one source fails",
);

assertTemplateOrder(
  learnTemplate,
  [
    'class="learn-hero nx-page-hero"',
    'class="classroom-entry card ios-card learn-section nx-panel section"',
    'class="card ios-card learn-section nx-panel section teacher-section"',
    'class="card ios-card learn-section nx-panel section courseware-section"',
    'class="card ios-card learn-section nx-panel section type-section"',
    'class="card ios-card learn-section nx-panel section quote-section"',
  ],
  "teacher classroom learning page",
);
assert.match(learnTemplate, /class=["']learn-hero__eyebrow["']>\s*\{\{ learnCopy\.hero\.eyebrow \}\}/, "learn hero should render the configured external classroom name");
assert.match(
  learnTemplate,
  /<NxAsyncState\b(?=[^>]*state=["']stale["'])(?=[^>]*@action=["']retryContentRefresh["'])[^>]*>/,
  "learn should keep cached content visible with a stale retry state",
);
assert.match(
  learnTemplate,
  /<NxAsyncState\b(?=[^>]*v-else-if=["']loadError["'])(?=[^>]*state=["']error["'])(?=[^>]*@action=["']loadContent["'])[^>]*>/,
  "learn should expose a retryable first-load error state",
);
assert.match(
  learnTemplate,
  /<NxAsyncState\s+v-if=["']classroomLoading["']\s+state=["']loading["']\s*\/>/,
  "learn classroom preview should expose loading state",
);
assert.match(
  learnTemplate,
  /<NxAsyncState\b(?=[^>]*classroomWarning && classroomPreview\.length === 0)(?=[^>]*state=["']error["'])(?=[^>]*@action=["']retryClassroomPreview["'])[^>]*>/,
  "learn classroom preview should expose retryable total failure",
);
assert.match(
  learnTemplate,
  /<NxAsyncState\b(?=[^>]*classroomPreview\.length === 0)(?=[^>]*state=["']empty["'])[^>]*>/,
  "learn classroom preview should expose visitor-facing empty content",
);
assert.match(
  learnTemplate,
  /classroomWarning && classroomPreview\.length > 0[\s\S]*已加载的课堂内容仍可继续浏览/,
  "learn should preserve loaded classroom items during a partial refresh failure",
);

for (const { imageClass, errorHandler, fallbackClass } of [
  {
    imageClass: "classroom-entry__cover",
    errorHandler: "markClassroomPreviewCoverError",
    fallbackClass: "classroom-entry__cover--fallback",
  },
  {
    imageClass: "teacher-card__avatar",
    errorHandler: "markTeacherImageError",
    fallbackClass: "teacher-media__fallback",
  },
  {
    imageClass: "type-badge__avatar",
    errorHandler: "markTypeImageError",
    fallbackClass: "type-badge__fallback",
  },
]) {
  assert.match(
    learnTemplate,
    new RegExp(`<image\\b(?=[^>]*${imageClass})(?=[^>]*lazy-load)(?=[^>]*@error=["']${errorHandler})[^>]*\\/>[\\s\\S]*?<view\\s+v-else[^>]*${fallbackClass}`),
    `${imageClass} should lazy-load and expose an image-error fallback`,
  );
}
assert.match(
  learnTemplate,
  /<view\b(?=[^>]*class=["']classroom-entry__item["'])(?=[^>]*role=["']button["'])(?=[^>]*tabindex=["']0["'])(?=[^>]*@keydown\.enter=)(?=[^>]*@keydown\.space\.prevent=)[^>]*>/,
  "learn classroom cards should keep keyboard navigation semantics",
);
for (const selector of [
  ".classroom-entry__more",
  ".retry",
  ".classroom-entry__item",
  ".learn-cta",
]) {
  assert.match(
    cssDeclarationsForSelector(learnStyle, selector),
    /min-height:\s*(?:8[8-9]|9\d|[1-9]\d{2,})rpx\s*;/,
    `${selector} should keep at least an 88rpx touch target`,
  );
}
for (const token of [
  "--nx-brand-900",
  "--nx-brand-700",
  "--nx-accent-gold",
  "--nx-page-bg",
  "--nx-surface",
  "--nx-surface-soft",
  "--nx-text",
  "--nx-text-muted",
  "--nx-border",
]) {
  assert.match(learnStyle, new RegExp(`var\\(${token}\\)`), `learn should consume ${token}`);
}
assert.doesNotMatch(
  learnTemplate,
  /后台发布|你发布的视频/,
  "learn visitor copy should not expose administrator workflow wording",
);

const profilePage = readFileSync("src/pages/profile/profile.vue", "utf8");
const profileEditPage = readFileSync("src/pages/profile-edit/profile-edit.vue", "utf8");
assert.match(
  profilePage,
  /profileLoading/,
  "profile page should expose a loading state for non-blocking history fetch",
);
assert.match(
  profilePage,
  /v-if="profileLoading"/,
  "profile page should render loading placeholder before empty states",
);
assert.match(profilePage, /loadTicket/, "profile page should ignore stale concurrent loads");
assert.match(profilePage, /recordsError/, "profile records should expose a request failure state");
assert.match(
  profilePage,
  /bookingsError/,
  "profile bookings should expose a request failure state",
);
assert.match(
  profilePage,
  /v-else-if=["']recordsError["']/,
  "profile records should render failure state before empty state",
);
assert.match(
  profilePage,
  /v-else-if=["']bookingsError["']/,
  "profile bookings should render failure state before empty state",
);
assert.match(
  profilePage,
  /同步失败，重试/,
  "profile history failures should show retry copy instead of empty copy",
);
assert.match(
  profilePage,
  /@click=["']loadAll["']/,
  "profile history failure state should provide a retry action",
);
assert.match(
  profilePage,
  /\.sync-retry\s*\{[\s\S]*min-height:\s*88rpx/,
  "profile history retry action should keep an 88rpx touch target",
);
assert.doesNotMatch(
  profilePage,
  /listTestRecordsApi\(\)\.catch\(\(\)\s*=>\s*\(\{\s*items:\s*\[\]\s*\}\)\)/,
  "profile records request failure must not be converted into an empty list",
);
assert.doesNotMatch(
  profilePage,
  /listBookingsApi\(\)\.catch\(\(\)\s*=>\s*\(\{\s*items:\s*\[\]\s*\}\)\)/,
  "profile bookings request failure must not be converted into an empty list",
);

const testPage = readFileSync("src/pages/test/test.vue", "utf8");

function vueSection(source, tagName) {
  if (tagName === "template") return topLevelVueSection(source, tagName);
  return source.match(new RegExp(`<${tagName}\\b[^>]*>([\\s\\S]*?)<\\/${tagName}>`))?.[1];
}

function quoteAwareTagEnd(source, startIndex) {
  let quote = null;
  for (let index = startIndex; index < source.length; index += 1) {
    const character = source[index];
    if (quote && character === "\\") {
      index += 1;
      continue;
    }
    if ((character === '"' || character === "'") && (!quote || quote === character)) {
      quote = quote ? null : character;
      continue;
    }
    if (character === ">" && !quote) return index;
  }
  return -1;
}

function topLevelVueSection(source, tagName) {
  const escapedTagName = tagName.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const opening = new RegExp(`^[ \\t]*<${escapedTagName}\\b`, "im").exec(source);
  if (!opening) return undefined;
  const openingEnd = quoteAwareTagEnd(source, opening.index);
  if (openingEnd < 0) return undefined;

  const openingPattern = new RegExp(`^<${escapedTagName}\\b`, "i");
  const closingPattern = new RegExp(`^<\\/${escapedTagName}\\s*>`, "i");
  let depth = 1;
  let cursor = openingEnd + 1;
  while (cursor < source.length) {
    if (source.startsWith("<!--", cursor)) {
      const commentEnd = source.indexOf("-->", cursor + 4);
      if (commentEnd < 0) return undefined;
      cursor = commentEnd + 3;
      continue;
    }
    if (source[cursor] !== "<") {
      cursor += 1;
      continue;
    }
    const remainder = source.slice(cursor);
    const closing = closingPattern.exec(remainder);
    if (closing) {
      depth -= 1;
      if (depth === 0) return source.slice(openingEnd + 1, cursor);
      cursor += closing[0].length;
      continue;
    }
    if (openingPattern.test(remainder)) {
      const nestedEnd = quoteAwareTagEnd(source, cursor);
      if (nestedEnd < 0) return undefined;
      if (!/\/\s*>$/.test(source.slice(cursor, nestedEnd + 1))) depth += 1;
      cursor = nestedEnd + 1;
      continue;
    }
    cursor += 1;
  }
  return undefined;
}

function stripMarkupAndCssComments(source) {
  return source.replace(/<!--[\s\S]*?-->/g, "").replace(/\/\*[\s\S]*?\*\//g, "");
}

const testTemplate = stripMarkupAndCssComments(vueSection(testPage, "template") || "");
const testStyle = stripMarkupAndCssComments(vueSection(testPage, "style") || "");

function openingTagsFor(source, tagName) {
  const tags = [];
  const opening = new RegExp(`<${tagName}\\b`, "g");
  for (const match of source.matchAll(opening)) {
    const end = quoteAwareTagEnd(source, match.index);
    if (end >= 0) tags.push(source.slice(match.index, end + 1));
  }
  return tags;
}

function tagAttribute(tag, attribute) {
  const escapedAttribute = attribute.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return tag.match(new RegExp(`\\s${escapedAttribute}=(["'])(.*?)\\1`))?.[2];
}

const nestedTemplateFixture = `
<script setup>
const ready = true;
</script>
<template data-condition="score > 0">
  <!-- <button class="commented" :loading="ignored">不应计入</button> -->
  <template v-if="ready">
    <button class="inside" :loading="insideBusy" :disabled="insideBusy">内部按钮</button>
  </template>
  <button
    class="after"
    data-condition="score > 0"
    :loading="afterBusy"
    :disabled="afterBusy"
  >内部 template 后的按钮</button>
</template>
`;
const nestedTemplateSource = stripMarkupAndCssComments(
  topLevelVueSection(nestedTemplateFixture, "template") || "",
);
const nestedTemplateButtons = openingTagsFor(nestedTemplateSource, "button");
assert.equal(
  nestedTemplateButtons.length,
  2,
  "top-level SFC template extraction should include nested-template content and ignore comments",
);
assert.ok(
  nestedTemplateButtons.some((button) => tagAttribute(button, "class") === "after"),
  "buttons after an internal template block should remain visible to global scans",
);
assert.match(
  nestedTemplateButtons.find((button) => tagAttribute(button, "class") === "after") || "",
  /data-condition=["']score > 0["']/,
  "quote-aware tag scanning should preserve greater-than signs inside attributes",
);
assert.doesNotMatch(
  nestedTemplateSource,
  /class=["']commented["']/,
  "commented buttons should not participate in global scans",
);

function pageStyleDeclarationBlocks(source, selector) {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return [...source.matchAll(new RegExp(`^[ \\t]*${escapedSelector}\\s*\\{([^}]*)\\}`, "gm"))].map(
    (match) => match[1],
  );
}

function pageStyleDeclarations(source, selector) {
  return pageStyleDeclarationBlocks(source, selector).at(-1);
}

function sourceBracedBody(source, match) {
  if (!match) return undefined;
  const openingBrace = match.index + match[0].lastIndexOf("{");
  let depth = 0;
  for (let index = openingBrace; index < source.length; index += 1) {
    if (source[index] === "{") depth += 1;
    if (source[index] !== "}") continue;
    depth -= 1;
    if (depth === 0) return source.slice(openingBrace + 1, index);
  }
  return undefined;
}

function hexToRgb(hex) {
  const normalized = hex.replace("#", "");
  const expanded =
    normalized.length === 3
      ? normalized
          .split("")
          .map((character) => character + character)
          .join("")
      : normalized;
  return [0, 2, 4].map((offset) => Number.parseInt(expanded.slice(offset, offset + 2), 16) / 255);
}

function relativeLuminance(hex) {
  return hexToRgb(hex)
    .map((channel) => (channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4))
    .reduce((sum, channel, index) => sum + channel * [0.2126, 0.7152, 0.0722][index], 0);
}

function contrastRatio(foreground, background) {
  const lighter = Math.max(relativeLuminance(foreground), relativeLuminance(background));
  const darker = Math.min(relativeLuminance(foreground), relativeLuminance(background));
  return (lighter + 0.05) / (darker + 0.05);
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
`;
const uncommentedFixtureTemplate = stripMarkupAndCssComments(
  vueSection(commentedSourceFixture, "template") || "",
);
const uncommentedFixtureStyle = stripMarkupAndCssComments(
  vueSection(commentedSourceFixture, "style") || "",
);
assert.equal(
  openingTagsFor(uncommentedFixtureTemplate, "button").length,
  0,
  "commented template controls must not satisfy UI contracts",
);
assert.equal(
  pageStyleDeclarationBlocks(uncommentedFixtureStyle, ".quiz__opt").length,
  0,
  "commented CSS rules must not satisfy visual contracts",
);

assert.match(testPage, /answerLocked/, "test page should guard rapid repeated option taps");
assert.match(testPage, /clearAdvanceTimer/, "test page should clear pending navigation timers");
assert.match(testPage, /onUnload/, "test page should clean up timers on unload");
assert.match(testPage, /const total = QUESTIONS\.length/, "test page should expose stable progress total");
assert.match(
  testTemplate,
  /class=["'][^"']*test-hero[^"']*nx-card/,
  "test game should use the shared expert-brand card surface",
);
assert.match(testTemplate, /九型测试小游戏/, "test game should use its external product name");
assert.match(testTemplate, /18 道生活情境题/, "test game should state its bounded question count");
assert.match(testTemplate, /约 3 分钟/, "test game should state its expected duration");

const progressContainer = testTemplate.match(
  /<view\b(?=[^>]*class=["'][^"']*quiz__progress-meta[^"']*["'])[^>]*>([\s\S]*?)<\/view>/,
);
assert.ok(progressContainer, "quiz should render progress metadata");
assert.match(
  progressContainer[0],
  /:aria-label=["']`第 \$\{step \+ 1\} 题，共 \$\{total\} 题`["']/,
  "quiz progress should announce current and total questions",
);
assert.match(progressContainer[1], /step \+ 1/, "quiz progress should render the current question");
assert.match(progressContainer[1], /total/, "quiz progress should render the total question count");

const testButtons = openingTagsFor(testTemplate, "button");
const genderButtons = testButtons.filter((tag) => staticClassTokens(tag).includes("gender__card"));
assert.equal(genderButtons.length, 2, "test should render two native identity buttons");
for (const { label, handler } of [
  { label: "选择男生并开始九型测试小游戏", handler: "start('male')" },
  { label: "选择女生并开始九型测试小游戏", handler: "start('female')" },
]) {
  const button = genderButtons.find((tag) => tagAttribute(tag, "@click") === handler);
  assert.ok(button, `test should preserve ${handler}`);
  assert.ok(staticClassTokens(button).includes("nx-focusable"), `${handler} should use shared focus behavior`);
  assert.equal(tagAttribute(button, "aria-label"), label, `${handler} should expose its purpose`);
}
assert.equal(
  new Set(genderButtons.map((tag) => tagAttribute(tag, "class"))).size,
  1,
  "identity choices should share one neutral visual treatment",
);

const quizOption = testButtons.find((tag) => tagAttribute(tag, "v-for") === "(opt, k) in q.options");
assert.ok(quizOption, "test should render quiz options as native buttons");
assert.equal(tagAttribute(quizOption, ":disabled"), "answerLocked", "quiz options should preserve the tap lock");
assert.equal(tagAttribute(quizOption, "@click"), "choose(opt)", "quiz options should preserve scoring navigation");
assert.match(tagAttribute(quizOption, ":aria-label") || "", /opt\.t/, "quiz options should describe answer text");
const quizBackButton = testButtons.find((tag) => staticClassTokens(tag).includes("quiz__back"));
assert.ok(quizBackButton, "quiz previous action should be a native button");
assert.equal(tagAttribute(quizBackButton, "@click"), "back", "quiz previous action should preserve its handler");

for (const selector of [".gender__card", ".quiz__opt", ".quiz__back"]) {
  const height = cssDeclarationsForSelector(testStyle, selector).match(/min-height:\s*(\d+)rpx\s*;/)?.[1];
  assert.ok(height && Number(height) >= 88, `${selector} should keep at least an 88rpx touch target`);
}
for (const token of [
  "--nx-brand-900",
  "--nx-brand-700",
  "--nx-accent-gold",
  "--nx-page-bg",
  "--nx-surface",
  "--nx-surface-soft",
  "--nx-text",
  "--nx-text-muted",
  "--nx-border",
]) {
  assert.match(testStyle, new RegExp(`var\\(${token}\\)`), `test game should consume ${token}`);
}
assert.match(
  testStyle,
  /@media\s*\(prefers-reduced-motion:\s*reduce\)\s*\{[\s\S]*?transition:\s*none\s*;/,
  "test game should disable interaction transitions for reduced motion",
);
const compactMedia = sourceBracedBody(
  testStyle,
  /@media\s*\(max-width:\s*360px\)\s*\{/.exec(testStyle),
);
assert.ok(compactMedia, "test game should keep its compact typography breakpoint");
assert.doesNotMatch(
  compactMedia,
  /\b(?:min-)?height\s*:/,
  "compact test layout must not reduce touch targets",
);

assert.match(
  learnPage,
  /loadContent\(\{\s*silent:\s*hasCachedContent\s*\}\)/,
  "learn should refresh silently after rendering cached content",
);
assert.match(
  learnPage,
  /if\s*\(silent\s*&&\s*!hasSiteConfigLearningSection\(cfg\)\)\s*return/,
  "a content-less background refresh should preserve cached learning content",
);
assert.match(
  learnPage,
  /if\s*\(silent\)[\s\S]*refreshError\.value = userErrorMessage/,
  "a background refresh failure should become a stale-content notice",
);
for (const stateName of ["teacherImageErrors", "typeImageErrors"]) {
  assert.match(
    learnPage,
    new RegExp(`const\\s+${stateName}\\s*=\\s*ref\\(\\{\\}\\)`),
    `learn should keep independent ${stateName} state`,
  );
}
assert.match(
  learnPage,
  /function\s+teacherMediaKey\s*\(teacher,\s*index\)[\s\S]*teacher\.name[\s\S]*teacher\.avatar[\s\S]*index/,
  "teacher image fallback identity should include content and list position",
);
assert.match(
  learnPage,
  /teacherImageErrors\.value\s*=\s*\{\s*\.\.\.teacherImageErrors\.value,\s*\[key\]:\s*true\s*\}/,
  "teacher image failures should update their keyed fallback state immutably",
);
assert.match(
  learnPage,
  /typeImageErrors\.value\s*=\s*\{\s*\.\.\.typeImageErrors\.value,\s*\[id\]:\s*true\s*\}/,
  "type image failures should update their keyed fallback state immutably",
);
assert.match(
  learnTemplate,
  /<view\b(?=[^>]*v-for=["']\(teacher, teacherIndex\) in teachers["'])(?=[^>]*:key=["']teacherMediaKey\(teacher, teacherIndex\)["'])[^>]*>/,
  "teacher cards should share the exact composite key used by avatar fallback state",
);
assert.match(
  learnTemplate,
  /<image\b(?=[^>]*class=["']teacher-media teacher-card__avatar["'])(?=[^>]*v-if=["']teacher\.avatar && !teacherImageErrors\[teacherMediaKey\(teacher, teacherIndex\)\]["'])(?=[^>]*@error=["']markTeacherImageError\(teacherMediaKey\(teacher, teacherIndex\)\)["'])[^>]*\/>/,
  "teacher avatars should read and write failure state through the same composite key",
);
assert.match(
  learnTemplate,
  /<view\s+v-else\s+class=["']courseware-list["']>[\s\S]*<view\b(?=[^>]*v-for=["']\(c, i\) in coursewareItems["'])(?=[^>]*:key=["']`\$\{c\.title \|\| ''\}:\$\{i\}`["'])(?![^>]*@click)[^>]*>/,
  "configured course direction cards should remain display-only inside an explicit list container",
);
assert.match(
  learnTemplate,
  /class=["']courseware-card__mark["'][^>]*aria-hidden=["']true["'][^>]*>\{\{\s*c\.badge\s*\|\|\s*i\s*\+\s*1\s*\}\}/,
  "course directions should use lightweight text marks instead of image layers",
);
assert.doesNotMatch(
  learnTemplate,
  /courseware-card__cover|course-media__fallback|markCourseImageError/,
  "course direction cards should avoid native image layers that can leave blank WeChat scroll regions",
);
assert.match(
  learnTemplate,
  /<view\b(?=[^>]*v-for=["']t in types["'])(?=[^>]*:key=["']t\.id["'])[^>]*>/,
  "type cards should use the same stable type id as their image fallback state",
);
assert.match(
  learnTemplate,
  /<image\b(?=[^>]*class=["']type-badge__avatar["'])(?=[^>]*v-if=["']!typeImageErrors\[t\.id\]["'])(?=[^>]*@error=["']markTypeImageError\(t\.id\)["'])[^>]*\/>/,
  "type avatars should read and write failure state through t.id",
);
assert.match(
  learnTemplate,
  /<button\b(?=[^>]*class=["'][^"']*learn-cta[^"']*["'])(?=[^>]*@click=["']goTest["'])[^>]*>/,
  "learn CTA should preserve its test navigation behavior",
);
assert.match(
  sourceBracedBody(learnPage, /function\s+goTest\s*\(\s*\)\s*\{/.exec(learnPage)) || "",
  /^\s*uni\.switchTab\(\{\s*url:\s*["']\/pages\/index\/index["']\s*\}\)\s*;?\s*$/,
  "learn test CTA should keep its fixed tab navigation target",
);
assert.match(
  learnTemplate,
  /v-else-if=["']!loadError && quotes\.length === 0["'][^>]*state=["']empty["']/,
  "learn quotes should expose a non-error empty state",
);

const profileEditTemplateRaw = vueSection(profileEditPage, "template") || "";
const profileEditTemplate = stripMarkupAndCssComments(profileEditTemplateRaw);
const profileEditStyle = stripMarkupAndCssComments(vueSection(profileEditPage, "style") || "");
assertRootViewClasses(profileEditPage, "src/pages/profile-edit/profile-edit.vue", [
  "page-stack",
  "ios-page",
  "ios-safe-bottom",
]);
assert.match(
  profileEditTemplate,
  /class=["'][^"']*profile-edit-hero[^"']*nx-page-hero[^"']*["']/,
  "personal profile should open with the shared expert-brand hero",
);
assert.match(
  profileEditTemplate,
  /class=["'][^"']*profile-edit-panel[^"']*nx-panel[^"']*ios-card[^"']*["']/,
  "personal profile editing should use the shared panel surface",
);
assert.match(
  profileEditPage,
  /import\s*\{[^}]*getToken[^}]*clearToken[^}]*\}\s*from\s*["']\.\.\/\.\.\/utils\/auth["']/,
  "personal profile should use the shared token boundary",
);
assert.match(
  profileEditPage,
  /import\s*\{[^}]*getUserInfoApi[^}]*updateUserInfoApi[^}]*\}\s*from\s*["']\.\.\/\.\.\/api["']/,
  "personal profile should reuse the existing user GET and PUT APIs",
);
assert.match(
  profileEditPage,
  /const profileLoading = ref\(false\)/,
  "personal profile should track initial loading independently",
);
assert.match(
  profileEditPage,
  /const profileSyncing = ref\(false\)/,
  "personal profile should track WeChat synchronization independently",
);
assert.match(
  profileEditPage,
  /const profileSaving = ref\(false\)/,
  "personal profile should track saving independently",
);
for (const [state, operation] of [
  ["profileLoading", "loading"],
  ["profileSyncing", "synchronization"],
  ["profileSaving", "saving"],
]) {
  assert.match(
    profileEditPage,
    new RegExp(`if \\(\\s*${state}\\.value\\s*\\) return`),
    `personal profile ${operation} should reject duplicate work`,
  );
}
assert.match(
  profileEditPage,
  /let sessionGeneration = 0/,
  "personal profile should maintain a page-session generation",
);
assert.match(
  profileEditPage,
  /onHide\([^)]*invalidateProfileSession[^)]*\)/,
  "personal profile should invalidate stale work when hidden",
);
assert.match(
  profileEditPage,
  /onUnload\([^)]*invalidateProfileSession[^)]*\)/,
  "personal profile should invalidate stale work when unloaded",
);
assert.match(
  profileEditPage,
  /function\s+isCurrentProfileSession\(generation, token, error\)[\s\S]*generation !== sessionGeneration[\s\S]*token === currentToken[\s\S]*error\.requestToken === token/,
  "personal profile should accept only the current token or an auth failure from that same token",
);
const profileSessionGuardBody =
  sourceBracedBody(
    profileEditPage,
    /function\s+isCurrentProfileSession\(generation, token, error\)\s*\{/.exec(profileEditPage),
  ) || "";
assert.match(
  profileSessionGuardBody,
  /!currentToken[\s\S]*error\.authExpired[\s\S]*error\.requestToken === token/,
  "personal profile should recognize a concurrent auth failure after the same token was already cleared",
);
assert.doesNotMatch(
  profileSessionGuardBody,
  /authSessionCurrent/,
  "personal profile must not require this request to be the first concurrent 401 that cleared the token",
);

const profileEditAuthBody =
  sourceBracedBody(
    profileEditPage,
    /function\s+redirectToProfileLogin\s*\([^)]*\)\s*\{/.exec(profileEditPage),
  ) || "";
assert.match(
  profileEditAuthBody,
  /if \(authRedirected\) return[\s\S]*authRedirected = true/,
  "personal profile auth expiry should only redirect once",
);
assert.match(
  profileEditAuthBody,
  /clearToken\(\)/,
  "personal profile auth expiry should clear the local token",
);
assert.match(
  profileEditAuthBody,
  /uni\.showToast\(\{\s*title:\s*'登录已过期，请重新登录',\s*icon:\s*'none'\s*\}\)/,
  "personal profile auth expiry should show one clear login toast",
);
assert.match(
  profileEditAuthBody,
  /uni\.switchTab\(\{\s*url:\s*'\/pages\/profile\/profile'\s*\}\)/,
  "personal profile auth expiry should return to the profile tab",
);
assert.match(
  profileEditPage,
  /statusCode === 401[\s\S]*statusCode === 403/,
  "personal profile should recognize both 401 and 403 authentication failures",
);
assert.match(
  profileEditPage,
  /if \(!token\)\s*\{\s*redirectToProfileLogin\(\)\s*return\s*\}/,
  "personal profile should redirect before requesting when no token exists",
);

const profileEditLoadBody =
  sourceBracedBody(
    profileEditPage,
    /async\s+function\s+loadProfile\s*\(\s*\)\s*\{/.exec(profileEditPage),
  ) || "";
assert.match(
  profileEditLoadBody,
  /const loadedUser = await getUserInfoApi\(\)\s*if \(!isCurrentProfileSession\(generation, token\)\) return\s*user\.value = loadedUser/,
  "personal profile should guard the user GET response before mutating state",
);
assert.match(
  profileEditLoadBody,
  /isAuthFailure\(e\)[\s\S]*redirectToProfileLogin\(\)/,
  "personal profile should redirect after authenticated GET failures",
);
assert.match(
  profileEditLoadBody,
  /catch \(e\) \{\s*if \(!isCurrentProfileSession\(generation, token, e\)\) return/,
  "personal profile load failures, including auth failures, should reject an old token session before side effects",
);
assert.match(
  profileEditLoadBody,
  /finally \{\s*if \(isCurrentProfileSession\(generation, token\)\) profileLoading\.value = false/,
  "personal profile load completion should not mutate a replacement token session",
);
assert.match(
  profileEditTemplate,
  /v-if=["']profileLoading["'][^>]*aria-live=["']polite["']/,
  "personal profile should announce its loading state",
);
assert.match(
  profileEditTemplate,
  /v-else-if=["']loadError["'][\s\S]*@click=["']loadProfile["']/,
  "personal profile should expose a readable non-auth error and retry",
);

const wechatProfileBlocks = [...profileEditPage.matchAll(/\/\/ #ifndef H5([\s\S]*?)\/\/ #endif/g)]
  .map((match) => match[1])
  .join("\n");
assert.match(
  wechatProfileBlocks,
  /getWechatProfilePayload/,
  "the non-H5 profile implementation should keep WeChat one-click synchronization",
);
assert.match(
  wechatProfileBlocks,
  /async function syncWechatProfile/,
  "the non-H5 build should define the WeChat synchronization handler",
);
const profileEditSyncBody =
  sourceBracedBody(
    profileEditPage,
    /async\s+function\s+syncWechatProfile\s*\(\s*\)\s*\{/.exec(profileEditPage),
  ) || "";
assert.match(
  profileEditSyncBody,
  /^\s*if \(profileSyncing\.value\) return\s*if \(profileSaving\.value\) return/,
  "personal profile sync should not start while either profile PUT operation is active",
);
assert.match(
  profileEditSyncBody,
  /catch \(e\) \{\s*if \(!isCurrentProfileSession\(generation, token, e\)\) return/,
  "personal profile sync failures, including auth failures, should reject an old token session before side effects",
);
assert.match(
  profileEditSyncBody,
  /finally \{\s*if \(isCurrentProfileSession\(generation, token\)\) profileSyncing\.value = false/,
  "personal profile sync completion should not mutate a replacement token session",
);
const wechatTemplateBlocks = [
  ...profileEditTemplateRaw.matchAll(/<!-- #ifndef H5 -->([\s\S]*?)<!-- #endif -->/g),
]
  .map((match) => match[1])
  .join("\n");
assert.match(
  wechatTemplateBlocks,
  /open-type=["']chooseAvatar["'][^>]*@chooseavatar=["']onChooseAvatar["']/,
  "the WeChat profile page should use the current chooseAvatar capability",
);
assert.match(
  wechatTemplateBlocks,
  /type=["']nickname["'][^>]*@input=["']onNicknameInput["']/,
  "the WeChat profile page should use the current nickname input capability",
);
assert.match(
  wechatTemplateBlocks,
  /:loading=["']profileSyncing["'][^>]*:disabled=["']profileSyncing \|\| profileSaving["'][^>]*@click=["']syncWechatProfile["']/,
  "the WeChat sync button should lock while either profile PUT operation is active",
);
const h5ProfileEditBlocks = [
  ...profileEditTemplateRaw.matchAll(/<!-- #ifdef H5 -->([\s\S]*?)<!-- #endif -->/g),
]
  .map((match) => match[1])
  .join("\n");
assert.match(
  h5ProfileEditBlocks,
  /请在微信小程序内同步微信资料/,
  "H5 should explain where WeChat profile synchronization is available",
);
assert.match(
  h5ProfileEditBlocks,
  /<button\b[^>]*\bdisabled\b[^>]*>[^<]*微信[^<]*<\/button>/,
  "H5 should render disabled WeChat capability guidance",
);
assert.doesNotMatch(
  h5ProfileEditBlocks,
  /getUserProfile|chooseAvatar|chooseavatar|syncWechatProfile|onChooseAvatar/,
  "H5 template blocks must not bind WeChat-only handlers",
);
assert.match(
  h5ProfileEditBlocks,
  /type=["']text["'][^>]*@input=["']onNicknameInput["']/,
  "H5 should keep nickname editing available for an existing token",
);

const profileEditSaveBody =
  sourceBracedBody(
    profileEditPage,
    /async\s+function\s+saveProfile\s*\(\s*\)\s*\{/.exec(profileEditPage),
  ) || "";
assert.match(
  profileEditSaveBody,
  /^\s*if \(profileSaving\.value\) return\s*if \(profileSyncing\.value\) return/,
  "personal profile save should not start while either profile PUT operation is active",
);
assert.match(
  profileEditSaveBody,
  /normalizeWechatProfile\([\s\S]*nickname:\s*nicknameDraft\.value[\s\S]*avatar:\s*avatarDraft\.value/,
  "personal profile save should normalize the current nickname and avatar draft",
);
assert.match(
  profileEditSaveBody,
  /hasProfilePayload\(payload\)/,
  "personal profile save should reject an empty normalized payload",
);
assert.match(
  profileEditSaveBody,
  /const updatedUser = await updateUserInfoApi\(payload\)\s*if \(!isCurrentProfileSession\(generation, token\)\) return\s*user\.value = updatedUser\s*syncDraftFromUser\(\)/,
  "personal profile save should guard the PUT response and refresh the form in place",
);
assert.match(
  profileEditSaveBody,
  /catch \(e\) \{\s*if \(!isCurrentProfileSession\(generation, token, e\)\) return/,
  "personal profile save failures, including auth failures, should reject an old token session before side effects",
);
assert.match(
  profileEditSaveBody,
  /finally \{\s*if \(isCurrentProfileSession\(generation, token\)\) profileSaving\.value = false/,
  "personal profile save completion should not mutate a replacement token session",
);
assert.doesNotMatch(
  profileEditSaveBody,
  /uni\.(?:navigateBack|switchTab)\s*\(/,
  "successful profile save should remain on the dedicated page",
);
assert.match(
  profileEditTemplate,
  /:loading=["']profileSaving["'][^>]*:disabled=["']profileSaving \|\| profileSyncing["'][^>]*@click=["']saveProfile["']/,
  "personal profile save should lock while either profile PUT operation is active",
);
assert.doesNotMatch(
  profileEditPage,
  /setStorageSync|saveProfileDraft|loadProfileDraft/,
  "unsaved personal-profile edits should not be persisted across page exits",
);
assert.match(
  pageStyleDeclarations(profileEditStyle, ".profile-edit-hero"),
  /linear-gradient\(145deg,\s*var\(--nx-brand-900\),\s*var\(--nx-brand-700\)\)/,
  "personal profile hero should use the shared brand tokens",
);
assert.match(
  pageStyleDeclarations(profileEditStyle, ".profile-save"),
  /min-height:\s*88rpx\s*;/,
  "personal profile save should keep an 88rpx touch target",
);

assert.doesNotMatch(
  profilePage,
  /wechatLoginReady/,
  "profile overview should leave WeChat capability guidance to the dedicated profile page",
);
assert.doesNotMatch(
  profilePage,
  /open-type="chooseAvatar"/,
  "profile overview should leave avatar selection to the dedicated profile page",
);
assert.doesNotMatch(
  profilePage,
  /type="nickname"/,
  "profile overview should leave nickname editing to the dedicated profile page",
);
for (const removedProfileOverviewSymbol of [
  "normalizeWechatProfile",
  "hasProfilePayload",
  "getWechatProfilePayload",
  "updateUserInfoApi",
  "profileSaving",
  "nicknameDraft",
  "avatarDraft",
  "draftAvatarFailed",
  "syncDraftFromUser",
  "syncWechatProfile",
  "saveProfile",
  "onChooseAvatar",
  "onNicknameInput",
]) {
  assert.doesNotMatch(
    profilePage,
    new RegExp(`\\b${removedProfileOverviewSymbol}\\b`),
    `profile overview should not retain ${removedProfileOverviewSymbol}`,
  );
}
assert.doesNotMatch(
  profilePage,
  /open-type="getPhoneNumber"/,
  "未接通后端前，手机号授权入口不能对用户露出",
);
assert.doesNotMatch(
  profilePage,
  /@getphonenumber="onGetPhoneNumber"/,
  "未接通后端前，不应绑定可见手机号授权占位事件",
);
assert.match(
  profilePage,
  /#ifdef H5[\s\S]*请在微信小程序内登录[\s\S]*#endif/,
  "H5 profile login entry should be a disabled miniapp guidance instead of a failing WeChat login CTA",
);
assert.doesNotMatch(
  profilePage,
  /后端暂未开通|前端占位|占位/,
  "用户侧文案不能暴露手机号授权后端占位状态",
);
assert.doesNotMatch(
  profilePage,
  /openChatPage|goChat|clearChatMessages|问 AI|AI 对话/,
  "profile page must not expose or reset removed AI chat state",
);

const profileTemplate = stripMarkupAndCssComments(vueSection(profilePage, "template") || "");
const profileStyle = stripMarkupAndCssComments(vueSection(profilePage, "style") || "");
assert.match(
  profileTemplate,
  /class=["'][^"']*profile-hero[^"']*nx-page-hero[^"']*["']/,
  "profile should open with the shared growth hero",
);
assert.match(
  profilePage,
  /const recordCount = computed\(\(\) => records\.value\.length\)/,
  "profile summary should derive its record count from loaded records",
);
assert.match(
  profilePage,
  /const bookingCount = computed\(\(\) => bookings\.value\.length\)/,
  "profile summary should derive its booking count from loaded bookings",
);
assert.match(
  profilePage,
  /const recordCountLabel = computed\(\(\) => profileLoading\.value \|\| recordsError\.value \? ['"]—['"] : String\(recordCount\.value\)\)/,
  "profile record count should stay unknown while loading or failed",
);
assert.match(
  profilePage,
  /const bookingCountLabel = computed\(\(\) => profileLoading\.value \|\| bookingsError\.value \? ['"]—['"] : String\(bookingCount\.value\)\)/,
  "profile booking count should stay unknown while loading or failed",
);
assert.ok(
  (profileTemplate.match(/class=["'][^"']*profile-stat(?:\s|["'])/g) || []).length >= 3,
  "profile hero should present three growth statistics",
);
assert.match(
  profileTemplate,
  /class=["'][^"']*history-timeline[^"']*["']/,
  "profile test history should use a timeline",
);
assert.match(
  profileTemplate,
  /class=["'][^"']*history-item[^"']*["']/,
  "profile timeline should expose structured items",
);
assert.match(
  profileTemplate,
  /class=["'][^"']*history-item__dot[^"']*["']/,
  "profile timeline should expose a visible dot",
);
assert.match(
  profileTemplate,
  /class=["'][^"']*history-item__body[^"']*["']/,
  "profile timeline should keep content separate from its dot",
);
assert.match(
  profilePage,
  /const userAvatarFailed = ref\(false\)/,
  "profile should track user avatar failures",
);
assert.match(
  profilePage,
  /function onUserAvatarError\(\)\s*\{\s*userAvatarFailed\.value = true\s*\}/,
  "profile should replace failed user avatars",
);
assert.doesNotMatch(
  profilePage,
  /管理档案/,
  "profile must not invent an unsupported archive-management action",
);
assert.match(
  profileTemplate,
  /<text\s+class=["']profile-hero__title["']>[^<]+<\/text>/,
  "profile hero should lead with a visible growth-oriented title",
);
assert.doesNotMatch(
  profileTemplate,
  /class=["'][^"']*profile-edit[^"']*nx-panel[^"']*["']/,
  "profile overview should not duplicate the dedicated profile editor",
);
assert.match(
  profileTemplate,
  /class=["'][^"']*history-section[^"']*nx-panel[^"']*["']/,
  "test history should use a shared panel surface",
);
assert.match(
  profileTemplate,
  /class=["'][^"']*booking-summary[^"']*nx-panel[^"']*["']/,
  "appointment summary should use a non-interactive shared panel shell",
);
assert.match(
  profileTemplate,
  /<view class=["']history-section nx-panel ios-card["']>\s*<view class=["']section-head["']>[\s\S]*?<text class=["']sec-title["']>我的测试历史<\/text>/,
  "test history content should belong to the history panel",
);
assert.match(
  profileTemplate,
  /<view class=["']booking-summary nx-panel ios-card["']>\s*<view class=["']section-head["']>[\s\S]*?<text class=["']sec-title["']>我的预约<\/text>/,
  "appointment content should belong to the booking summary shell",
);
assert.match(
  profileTemplate,
  /v-if=["']profileLoading["'][\s\S]*v-else-if=["']recordsError["'][\s\S]*v-else-if=["']records\.length === 0["'][\s\S]*v-else class=["']history-timeline["']/,
  "profile records should keep loading, error, empty, and timeline precedence",
);

const profileViews = openingTagsFor(profileTemplate, "view");
const profileIdentityActions = profileViews.filter((tag) =>
  staticClassTokens(tag).includes("profile-hero__identity-action"),
);
assert.equal(
  profileIdentityActions.length,
  1,
  "logged profile hero should expose one dedicated identity action",
);
assert.match(
  profileIdentityActions[0],
  /\saria-label=["']编辑个人资料["']/,
  "profile identity action should describe its destination",
);
assert.match(
  profileIdentityActions[0],
  /\s@click=["']openProfileEdit["']/,
  "profile identity action should bind profile navigation",
);
assert.match(
  profileIdentityActions[0],
  /\shover-class=["']profile-hero__identity-action--pressed["']/,
  "profile identity action should expose pressed feedback",
);
assertKeyboardViewControl(profileIdentityActions[0], "profile identity action", "openProfileEdit");
const profileIdentityActionStart = profileTemplate.indexOf(profileIdentityActions[0]);
const profileIdentityActionEnd = profileTemplate.indexOf(
  '<text class="profile-hero__title">',
  profileIdentityActionStart,
);
const profileIdentityActionContent = profileTemplate.slice(
  profileIdentityActionStart,
  profileIdentityActionEnd,
);
assert.match(
  profileIdentityActionContent,
  /<text\s+class=["']profile-hero__identity-arrow["']\s+aria-hidden=["']true["']>›<\/text>/,
  "profile identity action should include a decorative hidden right arrow",
);

const profileEditOpenBody =
  sourceBracedBody(profilePage, /function\s+openProfileEdit\s*\(\s*\)\s*\{/.exec(profilePage)) ||
  "";
assert.match(
  profileEditOpenBody,
  /uni\.navigateTo\s*\(\s*\{\s*url:\s*["']\/pages\/profile-edit\/profile-edit["']\s*\}\s*\)/,
  "profile identity action should navigate to the dedicated profile page",
);

const bookingSummaryShells = profileViews.filter((tag) =>
  staticClassTokens(tag).includes("booking-summary"),
);
assert.equal(bookingSummaryShells.length, 1, "profile should render one appointment summary shell");
for (const interactiveAttribute of [
  "role",
  "aria-role",
  "tabindex",
  "@click",
  "@keydown.enter",
  "@keydown.space.prevent",
]) {
  assert.equal(
    tagAttribute(bookingSummaryShells[0], interactiveAttribute),
    undefined,
    `appointment summary shell should not own ${interactiveAttribute}`,
  );
}
const bookingSummaryOpenTags = profileViews.filter((tag) =>
  staticClassTokens(tag).includes("booking-summary__open"),
);
assert.equal(
  bookingSummaryOpenTags.length,
  1,
  "appointment summary should expose one independent navigation body in every async state",
);
assert.match(
  bookingSummaryOpenTags[0],
  /\saria-label=["']查看全部预约记录["']/,
  "appointment summary action should describe its destination",
);
assert.match(
  bookingSummaryOpenTags[0],
  /\s@click=["']openBookingRecords["']/,
  "appointment summary action should bind records navigation",
);
assert.match(
  bookingSummaryOpenTags[0],
  /\shover-class=["']booking-summary__open--pressed["']/,
  "appointment summary action should expose pressed feedback",
);
assertKeyboardViewControl(
  bookingSummaryOpenTags[0],
  "appointment summary action",
  "openBookingRecords",
);
const bookingSummaryStatusTags = profileViews.filter((tag) =>
  staticClassTokens(tag).includes("booking-summary__status"),
);
assert.equal(
  bookingSummaryStatusTags.length,
  1,
  "appointment summary should expose one stable asynchronous status container",
);
assert.equal(
  tagAttribute(bookingSummaryStatusTags[0], "aria-live"),
  "polite",
  "appointment summary status changes should be announced politely",
);
assert.match(
  profileTemplate,
  /class=["']booking-summary__open["'][\s\S]*v-if=["']profileLoading["'][\s\S]*v-else-if=["']bookingsError["'][\s\S]*v-else-if=["']!latestBooking["'][\s\S]*v-else/,
  "appointment navigation body should remain present around loading, error, empty, and summary states",
);
assert.match(
  profileTemplate,
  /<button\s+v-if=["']bookingsError["']\s+class=["'][^"']*booking-summary__retry[^"']*["'][^>]*tabindex=["']0["'][^>]*@click\.stop=["']loadAll["'][^>]*>重试<\/button>/,
  "appointment retry should be an independently focusable native sibling button that only reloads",
);

assert.match(
  profilePage,
  /const latestBooking = computed\(\(\) => bookings\.value\[0\] \|\| null\)/,
  "profile appointment summary should use the first API item as the latest record",
);
assert.doesNotMatch(
  profilePage,
  /bookings\.value[^\n]*\.sort\s*\(|visibleBookings|hiddenBookingCount/,
  "profile appointment summary should preserve API order and avoid a local preview list",
);
const bookingRecordsOpenBody =
  sourceBracedBody(profilePage, /function\s+openBookingRecords\s*\(\s*\)\s*\{/.exec(profilePage)) ||
  "";
assert.match(
  bookingRecordsOpenBody,
  /uni\.navigateTo\s*\(\s*\{\s*url:\s*["']\/pages\/booking-records\/booking-records["']\s*\}\s*\)/,
  "profile appointment action should navigate to appointment records",
);
const h5ProfileLogin = profilePage.match(/<!-- #ifdef H5 -->([\s\S]*?)<!-- #endif -->/)?.[1] || "";
assert.match(
  h5ProfileLogin,
  /<button\b[^>]*\bdisabled\b[^>]*>请在微信小程序内登录<\/button>/,
  "H5 profile login guidance should remain disabled",
);
assert.doesNotMatch(
  h5ProfileLogin,
  /@click=["']login["']/,
  "H5 profile guidance must not call WeChat login",
);
const profileLoadAllBody =
  sourceBracedBody(profilePage, /async function\s+loadAll\s*\(\s*\)\s*\{/.exec(profilePage)) || "";
assert.match(
  profileLoadAllBody,
  /const requestToken = getToken\(\)/,
  "profile load should bind requests to the token that started them",
);
assert.match(
  profileLoadAllBody,
  /const loadedUser = await getUserInfoApi\(\)\s*if \(!isCurrentProfileLoad\(ticket, requestToken\)\)/,
  "profile load should reject a stale user response before mutating session state",
);
assert.match(
  profileLoadAllBody,
  /Promise\.allSettled\([\s\S]*?const historyAuthError[\s\S]*if \(!isCurrentProfileLoad\(ticket, requestToken, historyAuthError\)\)[\s\S]*if \(historyAuthError\)[\s\S]*handleAuthLoss\(ticket\)/,
  "profile load should reject stale history responses and centralize current auth failures",
);
assert.match(
  profilePage,
  /function\s+isCurrentProfileLoad\(ticket, token, error\)/,
  "profile should centralize current-token checks for all overview requests",
);
assert.match(
  profilePage,
  /function\s+invalidateStaleProfileLoad\(ticket = loadTicket\)/,
  "profile should isolate stale-token cleanup from auth reset",
);
assert.match(
  profilePage,
  /let sessionGeneration = 0/,
  "profile should maintain an independent authentication generation",
);
const profileLoginBody =
  sourceBracedBody(profilePage, /async function\s+login\s*\(\s*\)\s*\{/.exec(profilePage)) || "";
assert.match(
  profileLoginBody,
  /await ensureLogin\(\)\s*sessionGeneration \+= 1\s*generation = sessionGeneration\s*logged\.value = true[\s\S]*await loadAll\(\)\s*if \(!logged\.value \|\| generation !== sessionGeneration\) return\s*uni\.showToast\(\{ title: '登录成功'/,
  "profile login should establish a generation and suppress stale success feedback",
);
assert.match(
  profileLoginBody,
  /catch \(e\) \{\s*if \(generation !== sessionGeneration\) return[\s\S]*\}\s*finally \{\s*if \(generation === sessionGeneration\) logging\.value = false/,
  "profile login should suppress stale errors and protect newer login state",
);
const profileResetBody =
  sourceBracedBody(profilePage, /function\s+resetLogin\s*\(\s*\)\s*\{/.exec(profilePage)) || "";
assert.match(
  profileResetBody,
  /^\s*sessionGeneration \+= 1/,
  "profile reset should invalidate in-flight session work before clearing state",
);
assert.match(
  profileResetBody,
  /clearBookingSession\(\)/,
  "profile reset and logout should clear token-bound appointment detail state",
);
assert.match(
  profilePage,
  /const recordCountLabel = computed\(\(\) => profileLoading\.value \|\| recordsError\.value \? ['"]—['"] : String\(recordCount\.value\)\)/,
  "failed record counts should stay unknown instead of showing zero",
);
assert.match(
  profilePage,
  /const bookingCountLabel = computed\(\(\) => profileLoading\.value \|\| bookingsError\.value \? ['"]—['"] : String\(bookingCount\.value\)\)/,
  "failed booking counts should stay unknown instead of showing zero",
);

const profileHeroStyle = pageStyleDeclarations(profileStyle, ".profile-hero");
assert.match(
  profileHeroStyle,
  /linear-gradient\(145deg,\s*var\(--nx-brand-900\),\s*var\(--nx-brand-700\)\)/,
  "profile hero should use the shared brand tokens",
);
assert.match(
  profileHeroStyle,
  /border-radius:\s*38rpx\s*;/,
  "profile hero should use the approved radius",
);
assert.match(
  pageStyleDeclarations(profileStyle, ".profile-stats"),
  /grid-template-columns:\s*repeat\(3,\s*minmax\(0,\s*1fr\)\)\s*;/,
  "profile statistics should stay in three stable columns",
);
assert.match(
  pageStyleDeclarations(profileStyle, ".profile-stat__value"),
  /font-size:\s*34rpx\s*;[\s\S]*font-weight:\s*900\s*;/,
  "profile statistic values should keep the approved hierarchy",
);
assert.match(
  pageStyleDeclarations(profileStyle, ".history-item"),
  /min-height:\s*88rpx\s*;/,
  "profile history rows should keep a stable touch-friendly rhythm",
);
assert.match(
  pageStyleDeclarations(profileStyle, ".history-item__dot"),
  /width:\s*16rpx\s*;[\s\S]*height:\s*16rpx\s*;/,
  "profile timeline dots should use the approved fixed size",
);
assert.match(
  pageStyleDeclarations(profileStyle, ".logout"),
  /min-height:\s*88rpx\s*;/,
  "profile logout should keep an 88rpx touch target",
);
for (const selector of [".profile-hero__identity-action", ".booking-summary__open"]) {
  assert.match(
    pageStyleDeclarations(profileStyle, selector),
    /min-height:\s*88rpx\s*;/,
    `${selector} should keep an 88rpx touch target`,
  );
  assert.match(
    profileStyle,
    new RegExp(`${selector.replace(".", "\\.")}--pressed\\s*\\{[^}]*(?:opacity|transform)`),
    `${selector} should expose visible pressed feedback`,
  );
  assert.match(
    profileStyle,
    new RegExp(`${selector.replace(".", "\\.")}:focus-visible\\s*\\{[^}]*(?:outline|box-shadow)`),
    `${selector} should expose a visible focus state`,
  );
}
for (const selector of [
  ".profile-hero__eyebrow",
  ".profile-hero__lead",
  ".profile-stat__label",
  ".history-item__meta",
  ".more-tip",
]) {
  const fontSize = pageStyleDeclarations(profileStyle, selector)?.match(
    /font-size:\s*(\d+)rpx\s*;/,
  );
  assert.ok(
    fontSize && Number(fontSize[1]) >= 24,
    `${selector} should keep at least 24rpx readable text`,
  );
}
assert.match(
  pageStyleDeclarations(profileStyle, ".history-item__meta"),
  /color:\s*var\(--nx-text-muted\)\s*;/,
  "profile history metadata should use the shared readable secondary-text token",
);

const bookingRecordsPath = "src/pages/booking-records/booking-records.vue";
assert.ok(
  statSync(bookingRecordsPath, { throwIfNoEntry: false })?.isFile(),
  "appointment records page should exist",
);
const bookingRecordsPage = readFileSync(bookingRecordsPath, "utf8");
assert.match(
  bookingRecordsPage,
  /listBookingsApi/,
  "appointment records should use the authenticated booking list API",
);
assert.match(
  bookingRecordsPage,
  /getToken/,
  "appointment records should validate the current auth token",
);
assert.match(
  bookingRecordsPage,
  /clearToken/,
  "appointment records should clear auth after missing or expired authentication",
);
assert.match(
  bookingRecordsPage,
  /clearBookingSession/,
  "appointment records should clear token-bound booking state when auth changes",
);
assert.match(
  bookingRecordsPage,
  /setBookingSession\(currentToken,\s*record\)/,
  "appointment records should bind the selected record to the current token",
);
assert.match(
  bookingRecordsPage,
  /bookingKindLabel/,
  "appointment records should render Chinese booking kinds",
);
assert.match(
  bookingRecordsPage,
  /bookingStatusLabel/,
  "appointment records should render Chinese booking statuses",
);
assert.match(
  bookingRecordsPage,
  /maskBookingPhone/,
  "appointment records should mask phone numbers",
);
assert.doesNotMatch(
  bookingRecordsPage,
  /\.sort\s*\(/,
  "appointment records should preserve the API response order",
);
assert.match(
  bookingRecordsPage,
  /v-if=["']loading["']/,
  "appointment records should expose a loading state",
);
assert.match(
  bookingRecordsPage,
  /v-else-if=["']loadError["']/,
  "appointment records should expose an error state before empty state",
);
assert.match(
  bookingRecordsPage,
  /v-else-if=["']bookings\.length === 0["']/,
  "appointment records should expose an empty state",
);
assert.match(
  bookingRecordsPage,
  /aria-live=["']polite["']/,
  "appointment records async state should announce changes politely",
);
assert.match(
  bookingRecordsPage,
  /<button\s+class=["'][^"']*retry-button[^"']*["'][^>]*tabindex=["']0["'][^>]*@click\.stop=["']retryLoad["'][^>]*>/,
  "appointment records retry should be an independently focusable native button that stops propagation",
);
assert.match(
  bookingRecordsPage,
  /<button\s+class=["'][^"']*empty-action[^"']*["'][^>]*@click=["']goBooking["'][^>]*>去预约<\/button>/,
  "appointment records empty state should switch to the booking tab",
);
assert.match(
  bookingRecordsPage,
  /uni\.switchTab\s*\(\s*\{\s*url:\s*["']\/pages\/booking\/booking["']\s*\}\s*\)/,
  "appointment records empty action should switch to the booking tab",
);

const bookingRecordOpenTags =
  bookingRecordsPage.match(/<view\b[^>]*class=["'][^"']*booking-record__open[^"']*["'][^>]*>/g) ||
  [];
assert.ok(
  bookingRecordOpenTags.length > 0,
  "appointment records should render a dedicated navigation body",
);
for (const tag of bookingRecordOpenTags) {
  assert.match(
    tag,
    /\srole=["']button["']/,
    "appointment navigation body should use H5 button semantics",
  );
  assert.match(
    tag,
    /\saria-role=["']button["']/,
    "appointment navigation body should use WeChat button semantics",
  );
  assert.match(
    tag,
    /\stabindex=["']0["']/,
    "appointment navigation body should participate in keyboard focus order",
  );
  assert.match(
    tag,
    /\s@click=["']openBooking\(record\)["']/,
    "appointment navigation body should open its record",
  );
  assert.match(
    tag,
    /\s@keydown\.enter=["']openBooking\(record\)["']/,
    "appointment navigation body should activate with Enter",
  );
  assert.match(
    tag,
    /\s@keydown\.space\.prevent=["']openBooking\(record\)["']/,
    "appointment navigation body should activate with Space",
  );
}
assert.match(
  bookingRecordsPage,
  /uni\.navigateTo\s*\(\s*\{\s*url:\s*`\/pages\/booking-detail\/booking-detail\?id=\$\{[^}]+\}`[\s\S]*?\}\s*\)/,
  "appointment navigation should include the selected booking ID in the detail URL",
);

function vueFunctionBody(source, name) {
  const match = new RegExp(`(?:async\\s+)?function\\s+${name}\\s*\\([^)]*\\)\\s*\\{`).exec(source);
  if (!match) return undefined;
  const openingBrace = match.index + match[0].lastIndexOf("{");
  let depth = 0;
  for (let index = openingBrace; index < source.length; index += 1) {
    if (source[index] === "{") depth += 1;
    if (source[index] !== "}") continue;
    depth -= 1;
    if (depth === 0) return source.slice(openingBrace + 1, index);
  }
  return undefined;
}

const retryLoadBody = vueFunctionBody(bookingRecordsPage, "retryLoad");
assert.ok(
  retryLoadBody !== undefined,
  "appointment records should define an isolated retry handler",
);
assert.doesNotMatch(
  retryLoadBody,
  /setBookingSession|navigateTo|openBooking/,
  "retry must never set a booking session or navigate",
);
const authLossBody = vueFunctionBody(bookingRecordsPage, "handleAuthLoss");
assert.ok(
  authLossBody !== undefined,
  "appointment records should centralize authentication loss handling",
);
assert.match(authLossBody, /clearToken\(\)/, "authentication loss should clear auth");
assert.match(
  authLossBody,
  /clearBookingSession\(\)/,
  "authentication loss should clear booking session data",
);
assert.match(
  authLossBody,
  /redirecting/,
  "authentication loss should guard Toast and navigation side effects",
);
assert.match(
  authLossBody,
  /uni\.showToast/,
  "authentication loss should show one user-facing Toast",
);
assert.match(
  authLossBody,
  /uni\.switchTab/,
  "authentication loss should switch back to the profile tab",
);
assert.match(
  bookingRecordsPage,
  /loadTicket/,
  "appointment records should invalidate stale async responses",
);
const bookingSessionGuardBody = vueFunctionBody(bookingRecordsPage, "isCurrentBookingSession");
assert.ok(
  bookingSessionGuardBody !== undefined,
  "appointment records should centralize current-token session checks",
);
assert.match(
  bookingSessionGuardBody,
  /ticket !== loadTicket/,
  "appointment records should reject an old load generation",
);
assert.match(
  bookingSessionGuardBody,
  /token === currentToken/,
  "appointment records should accept responses only for the current token",
);
assert.match(
  bookingSessionGuardBody,
  /!currentToken[\s\S]*error\?\.authExpired[\s\S]*error\.requestToken === token/,
  "appointment records should recognize a current-session 401 after request cleanup",
);
const staleBookingBody = vueFunctionBody(bookingRecordsPage, "invalidateStaleBookingSession");
assert.ok(
  staleBookingBody !== undefined,
  "appointment records should isolate stale-token cleanup from auth redirect",
);
assert.match(
  staleBookingBody,
  /bookings\.value\s*=\s*\[\]/,
  "stale-token cleanup should remove the old user booking list",
);
assert.match(
  staleBookingBody,
  /clearBookingSession\(\)/,
  "stale-token cleanup should remove the old booking session",
);
assert.doesNotMatch(
  staleBookingBody,
  /clearToken|showToast|switchTab|navigateTo/,
  "stale-token cleanup must not mutate or redirect the newer auth session",
);
const bookingLoadBody = vueFunctionBody(bookingRecordsPage, "loadBookings");
assert.match(
  bookingLoadBody,
  /if \(!isCurrentBookingSession\(ticket, requestToken\)\)[\s\S]*invalidateStaleBookingSession\(ticket\)[\s\S]*loadedToken = requestToken/,
  "appointment success should reject an old token before exposing records",
);
assert.match(
  bookingLoadBody,
  /catch \(error\) \{\s*if \(!isCurrentBookingSession\(ticket, requestToken, error\)\)[\s\S]*invalidateStaleBookingSession\(ticket\)[\s\S]*if \(isAuthError\(error\)\)/,
  "appointment failures should reject an old token before auth side effects",
);
const openBookingBody = vueFunctionBody(bookingRecordsPage, "openBooking");
assert.match(
  openBookingBody,
  /if \(currentToken !== loadedToken\) \{\s*invalidateStaleBookingSession\(\)\s*return/,
  "clicking an old-token record should invalidate it without auth redirect",
);
assert.match(
  bookingRecordsPage,
  /statusCode\s*===\s*401[\s\S]*statusCode\s*===\s*403/,
  "appointment records should handle both 401 and 403",
);
assert.match(
  bookingRecordsPage,
  /onUnload/,
  "appointment records should invalidate loads and clear session on unload",
);
assert.match(
  bookingRecordsPage,
  /\.booking-record__open:focus-visible[\s\S]*(?:outline|box-shadow)/,
  "appointment navigation should expose a visible focus state",
);
assert.match(
  bookingRecordsPage,
  /\.booking-record__open\s*\{[\s\S]*min-height:\s*88rpx/,
  "appointment navigation should keep an 88rpx touch target",
);
for (const selector of [".booking-record__status", ".booking-record__meta"]) {
  const fontSize = pageStyleDeclarations(
    stripMarkupAndCssComments(vueSection(bookingRecordsPage, "style") || ""),
    selector,
  )?.match(/font-size:\s*(\d+)rpx\s*;/);
  assert.ok(
    fontSize && Number(fontSize[1]) >= 24,
    `${selector} should keep at least 24rpx readable text`,
  );
}
assert.match(
  openBookingBody,
  /uni\.navigateTo\s*\(\s*\{[\s\S]*fail\s*\([^)]*\)\s*\{[\s\S]*clearBookingSession\(\)/,
  "appointment detail navigation failure should clear the token-bound record session",
);

const bookingDetailPath = "src/pages/booking-detail/booking-detail.vue";
assert.ok(
  statSync(bookingDetailPath, { throwIfNoEntry: false })?.isFile(),
  "appointment detail page should exist",
);
const bookingDetailPage = readFileSync(bookingDetailPath, "utf8");
const bookingDetailTemplate = stripMarkupAndCssComments(
  vueSection(bookingDetailPage, "template") || "",
);
const bookingDetailStyle = stripMarkupAndCssComments(vueSection(bookingDetailPage, "style") || "");
assertRootViewClasses(bookingDetailPage, bookingDetailPath, [
  "page-stack",
  "ios-page",
  "ios-safe-bottom",
]);
assert.match(bookingDetailPage, /onLoad/, "appointment detail should read its route ID on load");
assert.match(
  bookingDetailPage,
  /onShow/,
  "appointment detail should revalidate auth and reload after returning from a hidden page",
);
assert.match(
  bookingDetailPage,
  /onHide/,
  "appointment detail should clear private fields as soon as it becomes hidden",
);
assert.match(
  bookingDetailPage,
  /normalizeBookingId\(query\?\.id\)/,
  "appointment detail should normalize untrusted route IDs safely",
);
assert.match(
  bookingDetailPage,
  /readBookingSession\(requestToken,\s*bookingId\)/,
  "appointment detail should try the token-bound booking session first",
);
assert.match(
  bookingDetailPage,
  /listBookingsApi\(\)/,
  "appointment detail should fall back to the existing booking list API",
);
assert.match(
  bookingDetailPage,
  /\.slice\(0,\s*50\)/,
  "appointment detail should inspect only the latest 50 booking records",
);
assert.match(
  bookingDetailPage,
  /String\(item\?\.id\)\s*===\s*bookingId/,
  "appointment detail fallback should compare numeric and string IDs equivalently",
);
assert.doesNotMatch(
  bookingDetailPage,
  /getBooking|bookingDetailApi|bookingByIdApi/,
  "appointment detail must not invent a new API",
);
assert.match(
  bookingDetailPage,
  /v-if=["']loading["']/,
  "appointment detail should expose a loading state",
);
assert.match(
  bookingDetailPage,
  /v-else-if=["']loadError["']/,
  "appointment detail should expose an error state before not-found",
);
assert.match(
  bookingDetailPage,
  /v-else-if=["']notFound["']/,
  "appointment detail should expose a dedicated not-found state",
);
assert.match(
  bookingDetailPage,
  /aria-live=["']polite["']/,
  "appointment detail async state should announce changes politely",
);
assert.match(
  bookingDetailTemplate,
  /@click=["']retryLoad["']/,
  "appointment detail error state should allow retrying",
);
assert.match(
  bookingDetailTemplate,
  /@click=["']goBookingRecords["']>返回预约列表<\/button>/,
  "appointment detail not-found state should return to the records list",
);
assert.match(
  bookingDetailPage,
  /uni\.redirectTo\s*\(\s*\{\s*url:\s*["']\/pages\/booking-records\/booking-records["']\s*\}\s*\)/,
  "appointment detail should reliably return to the records route",
);
for (const label of [
  "预约编号",
  "预约类型",
  "当前状态",
  "称呼",
  "手机号",
  "学习意向",
  "期望时间",
  "留言",
  "创建时间",
]) {
  assert.match(
    bookingDetailTemplate,
    new RegExp(`>${label}<\\/text>`),
    `appointment detail should show ${label}`,
  );
}
for (const field of [
  "id",
  "contactName",
  "phone",
  "intent",
  "preferredTime",
  "message",
  "createTime",
]) {
  assert.match(
    bookingDetailTemplate,
    new RegExp(`bookingValue\\(booking\\.${field}\\)`),
    `appointment detail should normalize empty ${field} values`,
  );
}
assert.match(
  bookingDetailTemplate,
  /bookingKindLabel\(booking\.kind\)/,
  "appointment detail should render the booking kind in Chinese",
);
assert.match(
  bookingDetailTemplate,
  /bookingStatusLabel\(booking\.status\)/,
  "appointment detail should render the booking status in Chinese",
);
assert.doesNotMatch(
  bookingDetailPage,
  /maskBookingPhone/,
  "appointment detail should show the complete phone number",
);

const detailLoadBody = vueFunctionBody(bookingDetailPage, "loadBookingDetail");
assert.ok(
  detailLoadBody !== undefined,
  "appointment detail should define an isolated loading function",
);
const invalidIdIndex = detailLoadBody.indexOf("if (!bookingId)");
const tokenIndex = detailLoadBody.indexOf("const requestToken = getToken()");
const listIndex = detailLoadBody.indexOf("await listBookingsApi()");
assert.ok(
  invalidIdIndex >= 0 && invalidIdIndex < tokenIndex && tokenIndex < listIndex,
  "appointment detail should reject an invalid ID before auth or API work",
);
assert.match(
  detailLoadBody,
  /readBookingSession\(requestToken,\s*bookingId\)[\s\S]*await listBookingsApi\(\)/,
  "appointment detail should use session data before falling back to the list",
);
const detailAuthLossBody = vueFunctionBody(bookingDetailPage, "handleAuthLoss");
assert.ok(
  detailAuthLossBody !== undefined,
  "appointment detail should centralize authentication loss handling",
);
assert.match(
  detailAuthLossBody,
  /clearToken\(\)/,
  "appointment detail auth loss should clear auth",
);
assert.match(
  detailAuthLossBody,
  /clearBookingSession\(\)/,
  "appointment detail auth loss should clear booking session data",
);
assert.match(
  detailAuthLossBody,
  /redirecting/,
  "appointment detail auth loss should guard Toast and navigation side effects",
);
assert.match(
  detailAuthLossBody,
  /uni\.showToast/,
  "appointment detail auth loss should show one user-facing Toast",
);
assert.match(
  detailAuthLossBody,
  /uni\.switchTab/,
  "appointment detail auth loss should switch to the profile tab",
);
const detailSessionGuardBody = vueFunctionBody(bookingDetailPage, "isCurrentBookingContext");
assert.ok(
  detailSessionGuardBody !== undefined,
  "appointment detail should centralize stale request checks",
);
assert.match(
  detailSessionGuardBody,
  /ticket !== loadTicket/,
  "appointment detail should reject an old load generation",
);
assert.match(
  detailSessionGuardBody,
  /bookingId !== routeBookingId/,
  "appointment detail should reject a response for another route ID",
);
assert.match(
  detailSessionGuardBody,
  /token === currentToken/,
  "appointment detail should accept only its current token",
);
assert.match(
  detailSessionGuardBody,
  /!currentToken[\s\S]*error\?\.authExpired[\s\S]*error\.requestToken === token/,
  "appointment detail should recognize a current-session auth failure after request cleanup",
);
const staleDetailBody = vueFunctionBody(bookingDetailPage, "invalidateStaleBookingContext");
assert.ok(
  staleDetailBody !== undefined,
  "appointment detail should isolate stale response cleanup",
);
assert.match(
  staleDetailBody,
  /booking\.value\s*=\s*null/,
  "appointment detail stale cleanup should hide old-user data",
);
assert.match(
  staleDetailBody,
  /clearBookingSession\(\)/,
  "appointment detail stale cleanup should clear booking session data",
);
assert.doesNotMatch(
  staleDetailBody,
  /clearToken|showToast|switchTab|navigateTo|redirectTo/,
  "appointment detail stale cleanup must not mutate or redirect the newer auth session",
);
assert.match(
  bookingDetailPage,
  /statusCode\s*===\s*401[\s\S]*statusCode\s*===\s*403/,
  "appointment detail should handle both 401 and 403",
);
assert.match(
  bookingDetailPage,
  /onUnload\s*\(\s*\(\)\s*=>\s*\{[\s\S]*loadTicket\s*\+=\s*1[\s\S]*clearBookingSession\(\)/,
  "appointment detail should invalidate pending work and clear session on unload",
);
assert.match(
  bookingDetailPage,
  /onHide\s*\(\s*\(\)\s*=>\s*\{[\s\S]*loadTicket\s*\+=\s*1[\s\S]*booking\.value\s*=\s*null[\s\S]*clearBookingSession\(\)/,
  "appointment detail should invalidate pending work and clear private detail on hide",
);
assert.match(
  pageStyleDeclarations(bookingDetailStyle, ".detail-action"),
  /min-height:\s*88rpx\s*;/,
  "appointment detail actions should keep an 88rpx touch target",
);

const resultPage = readFileSync("src/pages/result/result.vue", "utf8");
const resultTemplateRaw = resultPage.match(/<template>([\s\S]*)<\/template>\s*<style/)?.[1] || "";
const resultTemplate = stripMarkupAndCssComments(resultTemplateRaw);
const resultStyle = stripMarkupAndCssComments(vueSection(resultPage, "style") || "");
const resultViews = openingTagsFor(resultTemplate, "view");

assert.match(
  resultPage,
  /import\s*\{\s*reportDisplayState\s*\}\s*from\s*['"]\.\.\/\.\.\/utils\/reportDisplayState['"]/,
  "result page should use the pure report display-state helper",
);
assert.match(
  resultPage,
  /const reportPriceCents = ref\(null\)/,
  "result price should start unknown instead of assuming a charge",
);
assert.match(
  resultPage,
  /const reportStatusLoading = ref\(false\)/,
  "result page should track report status loading separately",
);
assert.match(
  resultPage,
  /const reportStatusError = ref\(['"]['"]\)/,
  "result page should expose report status errors",
);
assert.match(
  resultPage,
  /const reportState = computed\(\(\) => reportDisplayState\(\{[\s\S]*recordId:[\s\S]*loading:\s*reportStatusLoading\.value,[\s\S]*error:\s*reportStatusError\.value,[\s\S]*unlocked:\s*reportUnlocked\.value,[\s\S]*priceCents:\s*reportPriceCents\.value,[\s\S]*\}\)\)/,
  "result page should derive its report UI from the pure five-state helper",
);
assert.doesNotMatch(
  resultPage,
  /ref\(990\)/,
  "result page must not hard-code a default report price",
);

for (const className of [
  "result-hero",
  "drive-grid",
  "center-panel",
  "direction-grid",
  "report-panel",
]) {
  assert.ok(
    resultViews.some((tag) => staticClassTokens(tag).includes(className)),
    `result page should render ${className}`,
  );
}
assert.match(
  resultTemplate,
  /class=["'][^"']*result-hero[^"']*nx-page-hero[^"']*["']/,
  "result hero should use the shared hero surface",
);
assert.match(
  resultTemplate,
  /class=["']result-hero[^"']*["']\s+:class=["']`result-hero--\$\{info\.color\}`["']/,
  "result hero should use the personality color modifier",
);
assert.match(
  resultPage,
  /const avatarFailed = ref\(false\)/,
  "result page should track avatar load failure",
);
assert.match(
  resultPage,
  /import\s*\{\s*createResultPoster\s*\}\s*from\s*['"]\.\.\/\.\.\/utils\/resultPoster['"]/,
  "result page should delegate canvas drawing to the poster utility",
);
assert.doesNotMatch(
  resultPage,
  /function\s+drawPoster\s*\(/,
  "result page should not retain canvas drawing implementation",
);
assert.match(
  resultPage,
  /const posterError = ref\(['"]['"]\)/,
  "result page should expose recoverable poster errors",
);
const resultAvatar = openingTagsFor(resultTemplate, "image").find((tag) =>
  staticClassTokens(tag).includes("result-hero__avatar"),
);
assert.ok(resultAvatar, "result hero should render a fixed avatar image");
assert.equal(
  tagAttribute(resultAvatar, "v-if"),
  "!avatarFailed",
  "result avatar should be replaced after an image error",
);
assert.equal(
  tagAttribute(resultAvatar, "@error"),
  "avatarFailed = true",
  "result avatar should record image errors",
);
assert.match(resultAvatar, /\slazy-load(?:=|\s|>|$)/, "result avatar should lazy-load");
const resultAvatarFallback = resultViews.find((tag) =>
  staticClassTokens(tag).includes("result-hero__avatar-fallback"),
);
assert.ok(
  resultAvatarFallback && /\sv-else(?:\s|>|$)/.test(resultAvatarFallback),
  "result hero should render a mutually exclusive avatar fallback",
);
for (const selector of [".result-hero__avatar", ".result-hero__avatar-fallback"]) {
  const declarations = pageStyleDeclarations(resultStyle, selector);
  assert.match(declarations, /width:\s*184rpx\s*;/, `${selector} should reserve 184rpx width`);
  assert.match(declarations, /height:\s*184rpx\s*;/, `${selector} should reserve 184rpx height`);
}

const reportPanel = resultTemplate.match(
  /<view\s+class=["']report-panel["']>([\s\S]*?)<view\s+class=["']result-actions["']>/,
)?.[1];
assert.ok(reportPanel, "result page should expose one bounded report panel");
for (const state of ["needs-save", "status-loading", "status-error", "ready"]) {
  assert.match(
    reportPanel,
    new RegExp(`reportState\\.key === '${state}'`),
    `report panel should render ${state}`,
  );
}
assert.match(
  reportPanel,
  /<template\s+v-else>/,
  "report panel final branch should render the unlocked state",
);
assert.equal(
  (reportPanel.match(/@click=["']saveRecord["']/g) || []).length,
  1,
  "save should exist exactly once inside needs-save",
);
assert.equal(
  (reportPanel.match(/@click=["']unlockReport["']/g) || []).length,
  1,
  "unlock should exist exactly once inside ready",
);
assert.equal(
  (resultTemplate.match(/@click=["']saveRecord["']/g) || []).length,
  1,
  "result page should not duplicate its save CTA outside the report panel",
);
assert.equal(
  (resultTemplate.match(/@click=["']unlockReport["']/g) || []).length,
  1,
  "result page should not duplicate its unlock CTA outside the report panel",
);
assert.match(
  reportPanel,
  /aria-live=["']polite["'][^>]*>\s*查询报告状态/,
  "report status loading should be announced politely",
);
assert.match(
  reportPanel,
  /report__retry[^>]*@click=["']refreshReportStatus["']/,
  "report status failure should allow retrying status fetch",
);
assert.match(
  reportPanel,
  /report__content-retry[^>]*@click=["']loadReportContent["']/,
  "unlocked content failure should allow retrying content fetch",
);

const resultH5Blocks = [
  ...resultTemplateRaw.matchAll(/<!-- #ifdef H5 -->([\s\S]*?)<!-- #endif -->/g),
].map((match) => match[1]);
const h5SaveBlock = resultH5Blocks.find((block) => block.includes("请在微信小程序内登录后保存"));
assert.ok(h5SaveBlock, "H5 needs-save should explain that saving requires the miniapp");
assert.match(
  h5SaveBlock,
  /<button\b[^>]*\sdisabled(?:\s|>)/,
  "H5 save guidance should be disabled",
);
assert.doesNotMatch(h5SaveBlock, /@click=/, "H5 save guidance must not bind a save handler");
const h5PaymentBlock = resultH5Blocks.find((block) =>
  block.includes("请在微信小程序内完成存档与支付"),
);
assert.ok(h5PaymentBlock, "H5 ready state should explain that payment requires the miniapp");
assert.match(
  h5PaymentBlock,
  /<button\b[^>]*\sdisabled(?:\s|>)/,
  "H5 payment guidance should be disabled",
);
assert.doesNotMatch(
  h5PaymentBlock,
  /@click=/,
  "H5 payment guidance must not bind a payment handler",
);
const h5PosterBlock = resultH5Blocks.find((block) => block.includes("小程序内生成海报"));
assert.ok(
  h5PosterBlock && /\sdisabled(?:\s|>)/.test(h5PosterBlock),
  "H5 poster action should remain disabled guidance",
);
assert.doesNotMatch(
  resultH5Blocks.join("\n"),
  /open-type=["']share["']/,
  "H5 should not expose miniapp sharing",
);

const resultMpBlocks = [
  ...resultTemplateRaw.matchAll(/<!-- #ifdef MP-WEIXIN -->([\s\S]*?)<!-- #endif -->/g),
].map((match) => match[1]);
assert.ok(
  resultMpBlocks.some((block) => /open-type=["']share["']/.test(block)),
  "WeChat should preserve friend sharing",
);
assert.ok(
  resultMpBlocks.some((block) => /@click=["']saveRecord["']/.test(block)),
  "WeChat should preserve saving",
);
assert.ok(
  resultMpBlocks.some((block) => /@click=["']unlockReport["']/.test(block)),
  "WeChat should preserve report payment",
);
assert.ok(
  resultMpBlocks.some((block) =>
    /<button\b(?=[^>]*:loading=["']paying["'])(?=[^>]*:disabled=["']paying["'])(?=[^>]*@click=["']unlockReport["'])[^>]*>/.test(
      block,
    ),
  ),
  "WeChat report unlock should bind both loading and disabled state to the payment guard",
);

const refreshStatusBody = sourceBracedBody(
  resultPage,
  /async function\s+refreshReportStatus\s*\(\s*\)\s*\{/.exec(resultPage),
);
assert.match(
  refreshStatusBody,
  /reportStatusLoading\.value\s*=\s*true/,
  "report status refresh should enter loading state",
);
assert.match(
  refreshStatusBody,
  /reportStatusError\.value\s*=\s*['"]['"]/,
  "report status refresh should clear its prior error",
);
assert.match(
  refreshStatusBody,
  /reportUnlocked\.value\s*=\s*!!st\.unlocked/,
  "report status refresh should apply unlocked before validating price",
);
assert.match(
  refreshStatusBody,
  /Number\.isFinite\(st\.priceCents\)[\s\S]*st\.priceCents\s*>\s*0/,
  "locked report should accept only a finite positive price",
);
assert.match(
  refreshStatusBody,
  /finally\s*\{[\s\S]*reportStatusLoading\.value\s*=\s*false/,
  "report status refresh should always stop loading",
);
assert.match(
  resultPage,
  /const reportPriceYuan = computed\(\(\) => \{[\s\S]*Number\.isFinite\(reportPriceCents\.value\)[\s\S]*reportPriceCents\.value\s*>\s*0[\s\S]*return ''/,
  "report price display should stay blank until a valid positive price is known",
);

const saveRecordBody = sourceBracedBody(
  resultPage,
  /async function\s+saveRecord\s*\(\s*\)\s*\{/.exec(resultPage),
);
assert.match(
  saveRecordBody,
  /if\s*\(\s*!rec\s*\|\|\s*!rec\.id\s*\)\s*\{?\s*throw new Error\(['"]存档失败，请重试['"]\)/,
  "save should reject an API response without a record id",
);
const invalidRecordGuardIndex = saveRecordBody.indexOf("if (!rec || !rec.id)");
const assignRecordIdIndex = saveRecordBody.indexOf("recordId.value = rec.id");
const markSavedIndex = saveRecordBody.indexOf("saved.value = true");
const successToastIndex = saveRecordBody.indexOf(
  "uni.showToast({ title: '已存入我的档案', icon: 'success' })",
);
const refreshAfterSaveIndex = saveRecordBody.indexOf("await refreshReportStatus()");
assert.ok(
  invalidRecordGuardIndex >= 0 && invalidRecordGuardIndex < assignRecordIdIndex,
  "save should validate the record id before storing it",
);
assert.ok(
  assignRecordIdIndex < markSavedIndex,
  "save should store the valid record id before marking the result saved",
);
assert.ok(
  markSavedIndex < successToastIndex,
  "save should mark success before showing its success toast",
);
assert.ok(
  successToastIndex < refreshAfterSaveIndex,
  "save should acknowledge the valid archive before awaiting report status",
);
assert.match(
  saveRecordBody,
  /catch\s*\(e\)\s*\{[\s\S]*userErrorMessage\(e,\s*['"]存档失败，请重试['"]\)/,
  "save failures should use the normalized fallback message",
);
assert.match(
  saveRecordBody,
  /finally\s*\{[\s\S]*saving\.value\s*=\s*false/,
  "save should always restore its loading guard",
);

const loadReportBody = sourceBracedBody(
  resultPage,
  /async function\s+loadReportContent\s*\(\s*\)\s*\{/.exec(resultPage),
);
assert.match(
  loadReportBody,
  /if\s*\(reportLoading\.value\s*\|\|\s*reportContent\.value\)\s*return/,
  "report content loading should retain its duplicate-request guard",
);
const unlockReportBody = sourceBracedBody(
  resultPage,
  /async function\s+unlockReport\s*\(\s*\)\s*\{/.exec(resultPage),
);
assert.match(
  unlockReportBody,
  /^\s*if\s*\(paying\.value\)\s*return\s*paying\.value\s*=\s*true/,
  "report unlock should reject duplicate payment attempts before entering its loading state",
);
assert.match(
  unlockReportBody,
  /reportUnlocked\.value\s*=\s*true[\s\S]*loadReportContent\(\)/,
  "successful unlock should still load report content",
);
assert.match(
  unlockReportBody,
  /finally\s*\{\s*paying\.value\s*=\s*false\s*\}\s*$/,
  "report unlock should always release its payment guard",
);

const reportStyle = pageStyleDeclarations(resultStyle, ".report-panel");
assert.match(
  reportStyle,
  /background:\s*linear-gradient\([^;]+\)\s*;/,
  "report panel should keep a bounded high-contrast brand surface without locking gradient endpoints",
);
assert.match(
  reportStyle,
  /border-radius:\s*34rpx\s*;/,
  "report panel should use the planned 34rpx radius",
);
assert.match(
  reportStyle,
  /padding:\s*34rpx\s*;/,
  "report panel should use the planned 34rpx padding",
);
for (const selector of [
  ".report__cta",
  ".report__secondary",
  ".result-actions button",
  ".restart-button",
]) {
  const declarations = pageStyleDeclarations(resultStyle, selector);
  assert.match(
    declarations,
    /min-height:\s*88rpx\s*;/,
    `${selector} should keep an 88rpx touch target`,
  );
}
const reportButtonAlignmentRule = [
  ...resultStyle.matchAll(/^[ \t]*([^{}]+?)\s*\{([^{}]*)\}/gm),
].find(([, selectorText]) => {
  const selectors = new Set(selectorText.split(",").map((selector) => selector.trim()));
  return [".report__cta", ".report__secondary", ".result-actions button"].every((selector) =>
    selectors.has(selector),
  );
});
const reportButtonAlignmentDeclarations = reportButtonAlignmentRule?.[2];
assert.ok(
  reportButtonAlignmentDeclarations,
  "result report buttons should share one alignment CSS rule",
);
for (const [property, expected, description] of [
  ["display", "flex", "use flex layout for button-label alignment"],
  ["align-items", "center", "vertically center their button labels"],
  ["justify-content", "center", "horizontally center their button labels"],
  ["padding", "0 24rpx", "keep only horizontal 24rpx padding"],
  ["line-height", "1.2", "use the compact centered text line height"],
]) {
  const escapedExpected = expected.replace(".", "\\.");
  assert.match(
    reportButtonAlignmentDeclarations,
    new RegExp(`${property}:\\s*${escapedExpected.replace(" ", "\\s+")}\\s*;`),
    `shared report button alignment should ${description}`,
  );
}
for (const selector of [
  ".report__intro",
  ".report__status",
  ".report__error",
  ".report__content",
  ".disclaimer",
]) {
  const fontSize = pageStyleDeclarations(resultStyle, selector)?.match(/font-size:\s*(\d+)rpx\s*;/);
  assert.ok(
    fontSize && Number(fontSize[1]) >= 24,
    `${selector} should keep at least 24rpx readable text`,
  );
}
assert.match(
  pageStyleDeclarations(resultStyle, ".drive-grid"),
  /grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\)\s*;/,
  "result drives should stay in equal columns",
);
assert.match(
  pageStyleDeclarations(resultStyle, ".direction-grid"),
  /grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\)\s*;/,
  "result directions should stay in equal columns",
);
assert.match(
  resultTemplate,
  /aria-label=["']关闭海报["']/,
  "poster close action should expose an accessible label",
);
const posterDialog = resultTemplate.match(/<view\b[^>]*class=["']poster-box["'][^>]*>/)?.[0] || "";
assert.match(posterDialog, /\srole=["']dialog["']/, "poster surface should use dialog semantics");
assert.match(
  posterDialog,
  /\saria-modal=["']true["']/,
  "poster surface should identify itself as modal",
);
assert.match(
  resultTemplate,
  /poster-loading[^>]*aria-live=["']polite["']/,
  "poster generation should announce progress",
);
assert.match(
  resultTemplate,
  /poster-error[^>]*aria-live=["']polite["'][\s\S]*?@click=["']makePoster["']/,
  "poster failure should remain visible and retryable",
);
assert.match(
  resultTemplate,
  /v-if=["']posterUrl\s*&&\s*!posterLoading["'][^>]*@click=["']savePoster["']/,
  "poster save action should only exist after generation completes",
);
assert.match(
  resultPage,
  /userErrorMessage/,
  "result page should surface normalized request errors",
);
assert.match(
  resultPage,
  /normalizeLastResult/,
  "result page should validate cached result schema before rendering",
);
assert.match(
  resultPage,
  /测试结果已失效/,
  "result page should give feedback when cached result schema is invalid",
);

const relationPage = readFileSync("src/pages/relation/relation.vue", "utf8");
const relationTemplate = stripMarkupAndCssComments(
  relationPage.match(/<template>([\s\S]*)<\/template>\s*<style/)?.[1] || "",
);
const relationStyle = stripMarkupAndCssComments(vueSection(relationPage, "style") || "");

function elementBlocksByStaticClass(source, tagName, className) {
  const escapedTagName = tagName.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const tags = [...source.matchAll(new RegExp(`<\\/?${escapedTagName}\\b[^>]*>`, "g"))];
  const blocks = [];
  for (let startIndex = 0; startIndex < tags.length; startIndex += 1) {
    const openingTag = tags[startIndex][0];
    if (openingTag.startsWith("</") || openingTag.endsWith("/>")) continue;
    if (!staticClassTokens(openingTag).includes(className)) continue;
    let depth = 1;
    for (let endIndex = startIndex + 1; endIndex < tags.length; endIndex += 1) {
      const tag = tags[endIndex][0];
      if (tag.startsWith("</")) depth -= 1;
      else if (!tag.endsWith("/>")) depth += 1;
      if (depth !== 0) continue;
      blocks.push(source.slice(tags[startIndex].index, tags[endIndex].index + tag.length));
      break;
    }
  }
  return blocks;
}

const enneagramGameSource = readFileSync("src/data/enneagramGame.js", "utf8");
const typesInfoSource =
  enneagramGameSource.match(
    /export const TYPES_INFO\s*=\s*\{([\s\S]*?)\n\}\n\nexport const CENTERS/,
  )?.[1] || "";
const typeIds = [...typesInfoSource.matchAll(/^\s{2}([1-9]):\s*\{/gm)].map((match) =>
  Number(match[1]),
);
assert.deepEqual(
  typeIds,
  [1, 2, 3, 4, 5, 6, 7, 8, 9],
  "enneagram type data should expose every type id from 1 through 9",
);
assert.match(
  relationPage,
  /^const[ \t]+allTypes[ \t]*=[ \t]*Object\.keys\(TYPES_INFO\)\.map\([ \t]*\(id\)[ \t]*=>[ \t]*\(\{[ \t]*id:[ \t]*Number\(id\),[ \t]*\.\.\.TYPES_INFO\[id\][ \t]*\}\)[ \t]*\)[ \t]*;?[ \t]*$/m,
  "relation allTypes should map every TYPES_INFO entry without trailing slice or filter chains",
);
assert.match(
  relationPage,
  /<view\s+class=["'][^"']*page-stack[^"']*ios-page[^"']*ios-safe-bottom[^"']*["']/,
  "relation root should use shared page-stack/iOS safe-area classes",
);
assert.match(
  relationPage,
  /<button\s+class=["'][^"']*btn-primary[^"']*ios-button[^"']*["'][^>]*@click=["']analyze["']/,
  "relation primary action should opt into iOS button styling",
);
assert.doesNotMatch(
  relationPage,
  /padding-bottom:\s*60rpx/,
  "relation page should not hard-code bottom padding outside shared safe-area helpers",
);
const relationGridGap = relationPage.match(/\.grid\s*\{[\s\S]*?gap:\s*(\d+)rpx/);
assert.ok(
  relationGridGap && Number(relationGridGap[1]) >= 16,
  "relation type grid gap should be at least 16rpx",
);
assert.match(
  relationPage,
  /isValidTypeId/,
  "relation page should validate incoming and selected type ids",
);
assert.match(
  relationPage,
  /stage\.value\s*=\s*'redirecting'/,
  "relation invalid query should enter a redirecting state instead of leaving the pick UI interactive",
);
assert.match(
  relationPage,
  /v-else-if="stage === 'result'"/,
  "relation result view should be explicit so redirecting can show a safe placeholder",
);
assert.match(
  relationPage,
  /型号参数无效/,
  "relation page should explain invalid query type before navigation",
);
assert.match(
  relationPage,
  /\/pages\/test\/test/,
  "relation page should return to the test page for invalid query type",
);
assert.match(
  relationTemplate,
  /class=["'][^"']*relation-hero[^"']*nx-page-hero[^"']*["']/,
  "relation pick stage should use a themed hero",
);

const relationViews = openingTagsFor(relationTemplate, "view");
const typePickers = relationViews.filter((tag) => staticClassTokens(tag).includes("type-picker"));
assert.equal(typePickers.length, 2, "relation should render exactly two type pickers");
for (const picker of typePickers) {
  assert.ok(
    staticClassTokens(picker).includes("nx-panel"),
    "each relation type picker should use the shared panel surface",
  );
}

const relationButtons = openingTagsFor(relationTemplate, "button");
assert.equal(
  relationButtons.filter((tag) => tagAttribute(tag, "@click") === "analyze").length,
  1,
  "relation pick stage should keep exactly one primary analyze action",
);
const typeChips = relationButtons.filter((tag) => tagAttribute(tag, "v-for") === "t in allTypes");
assert.equal(
  typeChips.length,
  2,
  "relation should render one native type-chip loop for each person",
);
for (const { keyPrefix, ariaLabel, ariaPressed, handler } of [
  {
    keyPrefix: "'m' + t.id",
    ariaLabel: "`选择我的型号 ${t.id} ${t.name}`",
    ariaPressed: "myType === t.id",
    handler: "pickMy(t.id)",
  },
  {
    keyPrefix: "'t' + t.id",
    ariaLabel: "`选择 TA 的型号 ${t.id} ${t.name}`",
    ariaPressed: "taType === t.id",
    handler: "pickTa(t.id)",
  },
]) {
  const chip = typeChips.find((tag) => tagAttribute(tag, ":key") === keyPrefix);
  assert.ok(chip, `relation should render the ${keyPrefix} type-chip loop`);
  assert.equal(
    tagAttribute(chip, "class"),
    "type-chip nx-focusable",
    "relation type chips should use the exact shared focusable classes",
  );
  assert.equal(
    tagAttribute(chip, ":aria-label"),
    ariaLabel,
    "relation type chip should keep its accessible label on the native button",
  );
  assert.equal(
    tagAttribute(chip, ":aria-pressed"),
    ariaPressed,
    "relation type chip should expose its selected state on the native button",
  );
  assert.equal(
    tagAttribute(chip, "hover-class"),
    "type-chip--pressed",
    "relation type chip should expose pressed feedback on the native button",
  );
  assert.equal(
    tagAttribute(chip, "@click"),
    handler,
    "relation type chip should keep its selection handler on the native button",
  );
  assert.equal(
    tagAttribute(chip, "role"),
    "button",
    "relation type chip should expose H5 button semantics",
  );
  assert.equal(
    tagAttribute(chip, "aria-role"),
    "button",
    "relation type chip should expose miniapp button semantics",
  );
  assert.equal(
    tagAttribute(chip, "tabindex"),
    "0",
    "relation type chip should participate in H5 keyboard focus order",
  );
  assert.equal(
    tagAttribute(chip, "@keydown.enter"),
    handler,
    "relation type chip should activate with Enter",
  );
  assert.equal(
    tagAttribute(chip, "@keydown.space.prevent"),
    handler,
    "relation type chip should activate with Space without scrolling",
  );
}

for (const { handler, description } of [
  { handler: "analyze", description: "relation analyze action" },
  { handler: "reset", description: "relation reset action" },
]) {
  const button = relationButtons.find((tag) => tagAttribute(tag, "@click") === handler);
  assert.ok(button, `${description} should be a native button`);
  assert.equal(
    tagAttribute(button, "role"),
    "button",
    `${description} should expose H5 button semantics`,
  );
  assert.equal(
    tagAttribute(button, "aria-role"),
    "button",
    `${description} should expose miniapp button semantics`,
  );
  assert.equal(
    tagAttribute(button, "tabindex"),
    "0",
    `${description} should participate in H5 keyboard focus order`,
  );
  assert.equal(
    tagAttribute(button, "@keydown.enter"),
    handler,
    `${description} should activate with Enter`,
  );
  assert.equal(
    tagAttribute(button, "@keydown.space.prevent"),
    handler,
    `${description} should activate with Space without scrolling`,
  );
}

const chipBodies = [
  ...relationTemplate.matchAll(
    /<button\b(?=[^>]*class=["']type-chip nx-focusable["'])[^>]*>([\s\S]*?)<\/button>/g,
  ),
];
assert.equal(
  chipBodies.length,
  2,
  "relation should keep selected text inside both type-chip templates",
);
for (const [, body] of chipBodies) {
  assert.match(
    body,
    /<text\s+class=["']type-chip__number["']>\{\{ t\.id \}\}<\/text>/,
    "type chip should visibly render its number",
  );
  assert.match(
    body,
    /<text\s+class=["']type-chip__name["']>\{\{ t\.name \}\}<\/text>/,
    "type chip should visibly render its abbreviated name",
  );
  assert.match(
    body,
    /<text\s+v-if=["'][^"']+ === t\.id["']\s+class=["']type-chip__selected["']>已选<\/text>/,
    "selected chip should include a visible text marker",
  );
  assert.match(
    body,
    /<text\s+v-else\s+class=["']type-chip__selected type-chip__selected--placeholder["']\s+aria-hidden=["']true["']>/,
    "unselected chip should reserve the selected-marker layout slot",
  );
}

const typeChipStyle = pageStyleDeclarations(relationStyle, ".type-chip");
const typeChipHeight = typeChipStyle?.match(/height:\s*(\d+)rpx\s*;/);
const typeChipMinHeight = typeChipStyle?.match(/min-height:\s*(\d+)rpx\s*;/);
assert.ok(
  typeChipHeight && typeChipMinHeight,
  "relation type chips should define stable height and minimum height",
);
assert.equal(
  typeChipHeight[1],
  typeChipMinHeight[1],
  "selected and unselected relation chips should share one stable height",
);
assert.ok(
  Number(typeChipHeight[1]) >= 88,
  "relation type chips should keep at least an 88rpx touch target",
);
assert.match(
  typeChipStyle,
  /border-radius:\s*24rpx\s*;/,
  "relation type chips should keep the planned 24rpx radius",
);
assert.match(
  pageStyleDeclarations(relationStyle, ".type-chip__selected--placeholder"),
  /visibility:\s*hidden\s*;/,
  "unselected relation chips should reserve marker height without showing placeholder copy",
);
const selectedChipStyle = pageStyleDeclarations(relationStyle, ".type-chip.on");
assert.match(
  selectedChipStyle,
  /border:\s*4rpx\s+solid\s+var\(--nx-accent-gold\)\s*;/,
  "selected relation type should use the shared accent token",
);
assert.match(
  selectedChipStyle,
  /box-shadow:[^;]*\binset\b/i,
  "selected relation type should include a non-color-only inset emphasis",
);
assert.match(
  pageStyleDeclarations(relationStyle, ".type-chip--pressed"),
  /(?:opacity|transform)\s*:/,
  "relation chip press state should have visible feedback",
);

const relationHeroStyle = pageStyleDeclarations(relationStyle, ".relation-hero");
assert.match(
  relationHeroStyle,
  /linear-gradient\(135deg,\s*var\(--nx-brand-900\),\s*var\(--nx-brand-700\)\)/,
  "relation hero should use the shared brand gradient tokens",
);
for (const [selector, token] of [
  [".relation-hero__eyebrow", "--nx-accent-gold"],
  [".relation-hero__title", "--nx-surface"],
]) {
  assert.match(
    pageStyleDeclarations(relationStyle, selector),
    new RegExp(`color:\\s*var\\(${token}\\)\\s*;`),
    `${selector} should use ${token}`,
  );
}
assert.match(
  pageStyleDeclarations(relationStyle, ".relation-hero__desc"),
  /color:\s*rgba\(\s*255\s*,\s*255\s*,\s*255\s*,\s*\.(?:8\d|9\d)\s*\)\s*;/,
  "relation hero description should keep readable on-brand contrast",
);

assert.match(
  relationPage,
  /const myAvatarFailed = ref\(false\)/,
  "relation should track my avatar failure",
);
assert.match(
  relationPage,
  /const taAvatarFailed = ref\(false\)/,
  "relation should track TA avatar failure",
);
assert.match(
  sourceBracedBody(relationPage, /function\s+analyze\s*\(\s*\)\s*\{/.exec(relationPage)),
  /myAvatarFailed\.value\s*=\s*false[\s\S]*taAvatarFailed\.value\s*=\s*false/,
  "relation analyze should reset both avatar failure states",
);
assert.match(
  sourceBracedBody(relationPage, /function\s+reset\s*\(\s*\)\s*\{/.exec(relationPage)),
  /myAvatarFailed\.value\s*=\s*false[\s\S]*taAvatarFailed\.value\s*=\s*false/,
  "relation reset should clear both avatar failure states",
);

const relationImages = openingTagsFor(relationTemplate, "image");
assert.equal(
  relationImages.length,
  2,
  "relation result should render exactly two avatar image templates",
);
for (const { condition, errorHandler } of [
  { condition: "!myAvatarFailed", errorHandler: "onMyAvatarError" },
  { condition: "!taAvatarFailed", errorHandler: "onTaAvatarError" },
]) {
  const avatar = relationImages.find((tag) => tagAttribute(tag, "v-if") === condition);
  assert.ok(avatar, `relation should render the ${condition} avatar while available`);
  assert.ok(
    staticClassTokens(avatar).includes("pair__avatar"),
    "relation avatars should use the fixed avatar class",
  );
  assert.equal(
    tagAttribute(avatar, "@error"),
    errorHandler,
    "relation avatar should set its failure flag on image error",
  );
  assert.match(avatar, /\slazy-load(?:=|\s|>|$)/, "relation avatars should lazy-load");
}

const avatarFallbacks = relationViews.filter((tag) =>
  staticClassTokens(tag).includes("pair__avatar-fallback"),
);
assert.equal(avatarFallbacks.length, 2, "relation should render one fallback for each avatar");
for (const fallback of avatarFallbacks) {
  assert.ok(
    /\sv-else(?:\s|>|$)/.test(fallback),
    "relation avatar fallback should be mutually exclusive with its image",
  );
}
const avatarFallbackBlocks = elementBlocksByStaticClass(
  relationTemplate,
  "view",
  "pair__avatar-fallback",
);
assert.equal(
  avatarFallbackBlocks.length,
  2,
  "relation should expose two bounded avatar fallback elements",
);
assert.match(
  avatarFallbackBlocks[0],
  /^<view\b[^>]*>\{\{ myInfo\.id \}\}<\/view>$/,
  "my avatar fallback should display my type number within its own element",
);
assert.match(
  avatarFallbackBlocks[1],
  /^<view\b[^>]*>\{\{ taInfo\.id \}\}<\/view>$/,
  "TA avatar fallback should display TA type number within its own element",
);
for (const selector of [".pair__avatar", ".pair__avatar-fallback"]) {
  const declarations = pageStyleDeclarations(relationStyle, selector);
  assert.match(
    declarations,
    /width:\s*112rpx\s*;/,
    `${selector} should keep the planned fixed width`,
  );
  assert.match(
    declarations,
    /height:\s*112rpx\s*;/,
    `${selector} should keep the planned fixed height`,
  );
}

const pairHero = relationViews.find((tag) => staticClassTokens(tag).includes("pair"));
assert.ok(
  pairHero && staticClassTokens(pairHero).includes("nx-page-hero"),
  "relation result pair should use the shared hero container",
);
assert.ok(
  relationViews.some((tag) => staticClassTokens(tag).includes("pair-connection")),
  "relation result should render the connection visual",
);
const pairConnectionBlocks = elementBlocksByStaticClass(
  relationTemplate,
  "view",
  "pair-connection",
);
assert.equal(
  pairConnectionBlocks.length,
  1,
  "relation result should render exactly one bounded connection visual",
);
assert.match(
  pairConnectionBlocks[0],
  /<text\s+class=["']pair-connection__score["']>\{\{ analysis\.score \}\}<\/text>/,
  "connection visual should bind the analysis score inside its own container",
);
assert.match(
  pairConnectionBlocks[0],
  /<text\s+class=["']pair-connection__label["']>契合指数<\/text>/,
  "connection visual should label the score inside its own container",
);

const insightContracts = [
  { modifier: "insight--bond", binding: "analysis.bond" },
  { modifier: "insight--friction", binding: "analysis.friction" },
  { modifier: "insight--tip", binding: "analysis.tip" },
];
for (const { modifier, binding } of insightContracts) {
  assert.ok(
    relationViews.some((tag) => staticClassTokens(tag).includes(modifier)),
    `relation result should include ${modifier}`,
  );
  const blocks = elementBlocksByStaticClass(relationTemplate, "view", modifier);
  assert.equal(
    blocks.length,
    1,
    `relation result should render exactly one bounded ${modifier} panel`,
  );
  assert.match(
    blocks[0],
    new RegExp(
      `<text\\s+class=["']insight__text["']>\\{\\{ ${binding.replace(".", "\\.")} \\}\\}<\\/text>`,
    ),
    `${modifier} should bind ${binding} inside its own panel`,
  );
}
assert.ok(
  relationViews.some((tag) => staticClassTokens(tag).includes("drive-pair")),
  "relation drives should use a two-column container",
);
const drivePairBlocks = elementBlocksByStaticClass(relationTemplate, "view", "drive-pair");
assert.equal(
  drivePairBlocks.length,
  1,
  "relation should render exactly one bounded drive-pair container",
);
assert.match(
  drivePairBlocks[0],
  /\{\{ analysis\.myDrive \}\}/,
  "drive-pair should bind my drive inside its own container",
);
assert.match(
  drivePairBlocks[0],
  /\{\{ analysis\.taDrive \}\}/,
  "drive-pair should bind TA drive inside its own container",
);
assert.match(
  pageStyleDeclarations(relationStyle, ".drive-pair"),
  /grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\)\s*;/,
  "relation drives should remain two equal columns",
);
assert.match(
  pageStyleDeclarations(relationStyle, ".insight--bond"),
  /border-left:\s*6rpx\s+solid\s+var\(--nx-accent-gold\)\s*;/,
  "bond insight should use the shared accent token",
);
assert.match(
  pageStyleDeclarations(relationStyle, ".insight--friction"),
  /border-left:\s*6rpx\s+solid\s+var\(--nx-text-muted\)\s*;/,
  "friction insight should use the shared muted-text token",
);
assert.match(
  pageStyleDeclarations(relationStyle, ".insight--tip"),
  /border-left:\s*6rpx\s+solid\s+var\(--nx-brand-700\)\s*;/,
  "advice insight should use the shared brand token",
);
assert.match(
  pageStyleDeclarations(relationStyle, ".insight__text"),
  /color:\s*var\(--nx-text\)\s*;/,
  "relation insight body copy should use the shared primary-text token",
);
for (const { modifier } of insightContracts) {
  assert.match(
    pageStyleDeclarations(relationStyle, `.${modifier}`),
    /(?:background:\s*var\(--nx-surface\)|border-left:)/,
    `${modifier} should use a semantic surface or accent rule`,
  );
}

for (const { selector, minimum } of [
  { selector: ".relation-hero__eyebrow", minimum: 24 },
  { selector: ".type-picker__step", minimum: 24 },
  { selector: ".type-picker__hint", minimum: 24 },
  { selector: ".type-chip__name", minimum: 24 },
  { selector: ".type-chip__selected", minimum: 24 },
  { selector: ".pair__role", minimum: 24 },
  { selector: ".pair__name", minimum: 24 },
  { selector: ".pair-connection__eyebrow", minimum: 24 },
  { selector: ".pair-connection__label", minimum: 24 },
  { selector: ".insight__eyebrow", minimum: 24 },
  { selector: ".insight__text", minimum: 24 },
  { selector: ".drive__eyebrow", minimum: 24 },
  { selector: ".drive-card__label", minimum: 24 },
  { selector: ".drive-card__text", minimum: 24 },
  { selector: ".disclaimer", minimum: 24 },
]) {
  const fontSizeRule = pageStyleDeclarationBlocks(relationStyle, selector).find((declarations) =>
    /font-size:/.test(declarations),
  );
  const fontSize = fontSizeRule?.match(/font-size:\s*(\d+)rpx\s*;/);
  assert.ok(
    fontSize && Number(fontSize[1]) >= minimum,
    `${selector} should keep at least ${minimum}rpx readable text`,
  );
}

assert.doesNotMatch(
  relationTemplate,
  /✦|⚡|↗/,
  "relation insight icons should not depend on emoji or character glyphs",
);
for (const modifier of ["bond", "friction", "tip"]) {
  const marks = elementBlocksByStaticClass(relationTemplate, "view", `insight__icon--${modifier}`);
  assert.equal(marks.length, 1, `relation should render one CSS icon container for ${modifier}`);
  assert.match(
    marks[0],
    new RegExp(`<view\\s+class=["']insight__mark insight__mark--${modifier}["']\\s*\\/>`),
    `${modifier} insight should render a CSS-only mark`,
  );
  assert.ok(
    pageStyleDeclarations(relationStyle, `.insight__mark--${modifier}`),
    `${modifier} insight should define its CSS mark shape`,
  );
}

assert.match(
  resultPage,
  /<!--\s*#ifdef MP-WEIXIN\s*-->[\s\S]*@click="makePoster"[\s\S]*<!--\s*#endif\s*-->/,
  "poster generation must stay limited to mp-weixin",
);
assert.match(
  resultPage,
  /<!--\s*#ifdef H5\s*-->[\s\S]*<button[^>]*disabled[^>]*>小程序内生成海报<\/button>[\s\S]*<!--\s*#endif\s*-->/,
  "H5 poster entry should be a disabled compatibility hint",
);

assert.match(bookingPage, /loadBookingDraft/, "booking page should restore a local booking draft");
assert.match(
  bookingPage,
  /saveBookingDraft/,
  "booking page should auto-save booking draft changes",
);
assert.match(
  bookingPage,
  /clearBookingDraft/,
  "booking page should clear the local draft after successful submit",
);

console.log("ui compatibility tests passed");
