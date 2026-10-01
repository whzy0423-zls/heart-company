# Miniapp friend and Moments sharing — 2026-10-02

The native cover preview issue recorded below was subsequently fixed with published HTTPS covers; see [cover fix verification](miniapp-share-cover-fix-2026-10-02.md).

The teacher's public pages now offer native WeChat friend sharing and Moments sharing. Course/content links carry only public IDs; recipients use their own account and purchase permissions. This is a miniapp client change using the existing public database APIs. No backend migration or service restart is needed.

## Page policy

- Public: home, teacher, daily list, course introduction, classroom, lesson/video detail, nine-type overview/detail, public type result.
- Private: bookings/form/detail/list, orders, My Course, payment result, profile/editor, relationship analysis, unfinished assessment. Both native share menus are hidden on entry.
- Result cards contain only a validated type. A cold recipient receives a public type introduction, independent of saved assessments; the personal report and result ID are excluded.
- Friend button uses `open-type="share"`. Moments button explains the right-top native menu. WeChat's `onShareTimeline` supplies the current page's sanitized query, title and cover.
- Scene 1154 (Moments single-page preview) shows public content, skips private enrollment/progress/report requests, and guides visitors to “前往小程序” for actions requiring the full miniapp.
- Covers accept stable HTTPS or bundled PNG/JPG. Signed links, SVG/WebP and unsafe paths fall back to a bundled JPEG. The default studio card combines the existing teacher photo with the actual brand logo.

## Verification

Passed:

- `npm run test:config`: existing configuration, booking, payment, orders and My Course regressions plus the new sharing suites.
- Final `npm run test:sharing`: route/query allowlists, privacy, current metadata, async navigation/menu lifecycle, guest/authenticated recipient access, Timeline restrictions, malformed types and public result cold entry.
- `node src/pages/classroom/classroom.test.mjs`.
- `npm run build:mp-weixin`.
- `node scripts/share-policy.test.mjs --compiled`: all 19 page policies; both native hooks emitted for all 9 public pages.
- `git diff --check`.

Developer Tools Nightly 2.02.2606032 / WeChatLib 3.17.2:

- Opened the built project, saw both action buttons, and exercised the Moments instructions modal.
- Opened the native friend preview with the actual teacher title. Cancelled without sending.
- The native preview cover appeared blank. Console inspection returned the expected `/static/share/studio.jpg` payload, and `wx.getImageInfo` returned `getImageInfo:ok`, 500×400, JPEG, orientation up. The packaged image matches the source. This verifies asset presence/decoding, not final share rendering. No speculative path workaround was added.
- Reopened the project after diagnostics to restore the original page callback and clear temporary console state.
- Real-device friend/Moments cover display and receipt by another account remain to be checked. No messages or Moments posts were sent, and no WeChat public release was published.

Two optional broad checks have pre-existing failures and are not counted as passing:

- `classroom-detail.test.mjs`: its baseline source assertion rejects an existing nested avatar preview button. Existing playback/payment behavior checks passed independently.
- `release-regression-flow.test.mjs`: the baseline SFC loader lacks the `onResize` export imported by existing navigation code. The loader was left unchanged.

## Build for WeChat Developer Tools

Import `miniapp/dist/build/mp-weixin` from this repository. The build uses the configured production API. A website/backend deployment alone does not update an installed WeChat miniapp; release the client through the normal WeChat process after device acceptance.

Official single-page behavior reference: https://developers.weixin.qq.com/miniprogram/dev/framework/open-ability/share-timeline.html
