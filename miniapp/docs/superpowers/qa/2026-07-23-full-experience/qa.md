# Full miniapp experience visual QA

Date: 2026-07-23

The six redesigned H5 pages were checked at 360, 375, 390, and 800 CSS pixels. In every baseline combination, `scrollWidth === clientWidth`; hero hierarchy, primary actions, cards/grids, safe-bottom spacing, and state contrast remained readable. The 800 px captures were regenerated with `window.innerWidth === document.documentElement.clientWidth === 800` and device scale factor 1.

| Page/state | Full URL | Viewport | No horizontal overflow | Hero clear | Primary CTA clear | Grid/cards readable | Safe bottom visible | Contrast/states readable | Screenshot |
| --- | --- | ---: | --- | --- | --- | --- | --- | --- | --- |
| Test / gender | `http://127.0.0.1:4173/#/pages/test/test` | 360 | Pass | Pass | Pass | Pass | Pass | Pass | [test-gender-360.png](screenshots/test-gender-360.png) |
| Test / gender | `http://127.0.0.1:4173/#/pages/test/test` | 375 | Pass | Pass | Pass | Pass | Pass | Pass | [test-gender-375.png](screenshots/test-gender-375.png) |
| Test / gender | `http://127.0.0.1:4173/#/pages/test/test` | 390 | Pass | Pass | Pass | Pass | Pass | Pass | [test-gender-390.png](screenshots/test-gender-390.png) |
| Test / gender | `http://127.0.0.1:4173/#/pages/test/test` | 800 | Pass | Pass | Pass | Pass | Pass | Pass | [test-gender-800.png](screenshots/test-gender-800.png) |
| Result / public | `http://127.0.0.1:4173/#/pages/result/result` | 360 | Pass | Pass | Pass | Pass | Pass | Pass | [result-public-360.png](screenshots/result-public-360.png) |
| Result / public | `http://127.0.0.1:4173/#/pages/result/result` | 375 | Pass | Pass | Pass | Pass | Pass | Pass | [result-public-375.png](screenshots/result-public-375.png) |
| Result / public | `http://127.0.0.1:4173/#/pages/result/result` | 390 | Pass | Pass | Pass | Pass | Pass | Pass | [result-public-390.png](screenshots/result-public-390.png) |
| Result / public | `http://127.0.0.1:4173/#/pages/result/result` | 800 | Pass | Pass | Pass | Pass | Pass | Pass | [result-public-800.png](screenshots/result-public-800.png) |
| Relation / picker | `http://127.0.0.1:4173/#/pages/relation/relation?type=2` | 360 | Pass | Pass | Pass | Pass | Pass | Pass | [relation-picker-360.png](screenshots/relation-picker-360.png) |
| Relation / picker | `http://127.0.0.1:4173/#/pages/relation/relation?type=2` | 375 | Pass | Pass | Pass | Pass | Pass | Pass | [relation-picker-375.png](screenshots/relation-picker-375.png) |
| Relation / picker | `http://127.0.0.1:4173/#/pages/relation/relation?type=2` | 390 | Pass | Pass | Pass | Pass | Pass | Pass | [relation-picker-390.png](screenshots/relation-picker-390.png) |
| Relation / picker | `http://127.0.0.1:4173/#/pages/relation/relation?type=2` | 800 | Pass | Pass | Pass | Pass | Pass | Pass | [relation-picker-800.png](screenshots/relation-picker-800.png) |
| Learn / cached fixture | `http://127.0.0.1:4173/#/pages/learn/learn` | 360 | Pass | Pass | Pass | Pass | Pass | Pass | [learn-loaded-360.png](screenshots/learn-loaded-360.png) |
| Learn / cached fixture | `http://127.0.0.1:4173/#/pages/learn/learn` | 375 | Pass | Pass | Pass | Pass | Pass | Pass | [learn-loaded-375.png](screenshots/learn-loaded-375.png) |
| Learn / cached fixture | `http://127.0.0.1:4173/#/pages/learn/learn` | 390 | Pass | Pass | Pass | Pass | Pass | Pass | [learn-loaded-390.png](screenshots/learn-loaded-390.png) |
| Learn / cached fixture | `http://127.0.0.1:4173/#/pages/learn/learn` | 800 | Pass | Pass | Pass | Pass | Pass | Pass | [learn-loaded-800.png](screenshots/learn-loaded-800.png) |
| Booking / draft | `http://127.0.0.1:4173/#/pages/booking/booking` | 360 | Pass | Pass | Pass | Pass | Pass | Pass | [booking-draft-360.png](screenshots/booking-draft-360.png) |
| Booking / draft | `http://127.0.0.1:4173/#/pages/booking/booking` | 375 | Pass | Pass | Pass | Pass | Pass | Pass | [booking-draft-375.png](screenshots/booking-draft-375.png) |
| Booking / draft | `http://127.0.0.1:4173/#/pages/booking/booking` | 390 | Pass | Pass | Pass | Pass | Pass | Pass | [booking-draft-390.png](screenshots/booking-draft-390.png) |
| Booking / draft | `http://127.0.0.1:4173/#/pages/booking/booking` | 800 | Pass | Pass | Pass | Pass | Pass | Pass | [booking-draft-800.png](screenshots/booking-draft-800.png) |
| Profile / logged out | `http://127.0.0.1:4173/#/pages/profile/profile` | 360 | Pass | Pass | N/A — disabled H5 guidance clear | Pass | Pass | Pass | [profile-logged-out-360.png](screenshots/profile-logged-out-360.png) |
| Profile / logged out | `http://127.0.0.1:4173/#/pages/profile/profile` | 375 | Pass | Pass | N/A — disabled H5 guidance clear | Pass | Pass | Pass | [profile-logged-out-375.png](screenshots/profile-logged-out-375.png) |
| Profile / logged out | `http://127.0.0.1:4173/#/pages/profile/profile` | 390 | Pass | Pass | N/A — disabled H5 guidance clear | Pass | Pass | Pass | [profile-logged-out-390.png](screenshots/profile-logged-out-390.png) |
| Profile / logged out | `http://127.0.0.1:4173/#/pages/profile/profile` | 800 | Pass | Pass | N/A — disabled H5 guidance clear | Pass | Pass | Pass | [profile-logged-out-800.png](screenshots/profile-logged-out-800.png) |

Additional state evidence:

- [test-question-375.png](screenshots/test-question-375.png) records the active question state.
- [relation-result-375.png](screenshots/relation-result-375.png) records the generated relation result state.
- The learn screenshots use the plan's exact `nx_site_config_cache` teacher/course/quote fixture while `*/public/site-config*` is blocked. They demonstrate cached content appearing and remaining readable when the silent refresh fails; static tests cover the non-cached error branch.
- In stitched full-page screenshots, the fixed bottom tab bar can appear partway through a long image; this is a capture artifact, not an in-page layout position.
- At 800 CSS px the layouts expand cleanly with readable spacing and no horizontal overflow.

## Automated verification

- `node src/utils/reportDisplayState.test.mjs`: pass.
- `node scripts/ui-compat.test.mjs`: pass.
- The 15 independent follow-up test files listed in the implementation plan: pass individually.
- `npm run build:h5`: pass.
- `npm run build:mp-weixin`: pass.
- Generated WeChat contracts: sharing, result save/payment actions, two `.report__cta` `bindtap` actions, booking submit, `chooseAvatar`, and nickname input are present; H5-only disabled copy is absent.
- `git diff --check`: pass.
- Screenshot audit: exactly 26 files, all true PNG data; the six 800 px baseline captures report 800 CSS px client and inner width with no horizontal overflow.
- `npm run test:config`: known baseline failure at `scripts/project-config.test.mjs:36` because `.env.production.example` does not document a real HTTPS `VITE_API_BASE`. This pre-existing release-configuration issue was explicitly excluded from this redesign task; no claim is made that the aggregate command is green.

WeChat DevTools and physical-device checks remain release-gate activities per the repository release checklist; this QA package establishes code/build and H5 visual readiness, not production release approval.
