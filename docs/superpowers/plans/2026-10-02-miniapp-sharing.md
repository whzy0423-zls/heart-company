# Miniapp sharing implementation plan

**Goal:** Complete native WeChat friend and Timeline sharing for the teacher's public brand, course introductions and published daily media, while keeping customer/paid records private.

**Authorization:** The user approved the page policy proposed in the preceding audit and asked to implement both channels. Continue in the existing main branch.

**Architecture:** Page-level `onShareAppMessage` and `onShareTimeline` hooks ensure uni-app emits native runtime hooks. A pure payload builder accepts only public identifiers and metadata; a compact reusable action row invokes native friend sharing and explains the native Timeline menu. Payloads are read at share time, without login tokens, booking IDs, payment flags, personal reports or playback links. Existing database APIs remain authoritative for availability and recipient entitlement.

**Stack:** Vue/uni-app, native WeChat menus, Node behavioral tests.

Policy and landing behavior:

- Home, teacher, daily list: current public title/cover and canonical page route.
- Course introduction: course ID only; recipient's own ownership determines its enrollment CTA. Missing/disabled catalog does not enable sharing from private fallback data.
- Classroom and lesson: published series/content IDs, public title and cover. Existing playback access checks remain unchanged.
- Nine-type overview/detail: public educational data with a validated type.
- Test result: share only a validated type through `shareType`. The receiving result page renders a distinct public type introduction, with no dependency on either person's saved test result. Invalid/missing type in a result share becomes `shareType=0`, a generic public invitation. Personal result/report paths remain separate.
- Orders, private bookings, profile/editor, My Course, payment result and relationship analysis hide native share menus. Form data and unfinished test answers are not share payloads.
- Timeline uses the current page with a sanitized query, because WeChat does not let it override the page path. Its button explains the right-hand menu rather than claiming an automatic publish action. No message or Timeline post is sent during verification.

Files and work:

- [x] `utils/share.js`, `utils/share.test.mjs`: route/query allowlist, type/ID validation, title and image fallback, menu controls and Timeline instructions. Test identifier round trips and rejection of private fields/signed URLs.
- [x] `components/NxShareActions.vue`: compact friend/Timeline actions matching the existing studio styling, MP-only.
- [x] Public pages `index`, `teacher`, `learn`, `course-detail`: hooks, data readiness, public metadata and UI. Tests cover asynchronous metadata refresh and private-query exclusion.
- [x] Media pages `classroom`, `classroom-detail`: stable cold-entry links and real public metadata, unavailable-content menu handling, unchanged payment/playback access.
- [x] `result`, `enneagram`, `enneagram-detail`: cold friend/Timeline entry with and without recipient cache, malformed types, same-type re-share, normal report/poster regression.
- [x] Private-page `onShow` menu policy with no share callbacks.
- [x] Full configuration/behavior suite, relevant release media/result regressions and production WeChat build; inspect emitted hook flags and cover artifacts.
- [x] Reopen the developer-tool project and inspect friend/Timeline controls without sending a share. Record verification results; cold public landing is covered by behavioral tests.
- [ ] Real-device acceptance: check friend/Timeline cover rendering and open each shared card in another account, including Timeline scene 1154. Developer Tools showed a blank native cover despite successful runtime image decoding; do not treat it as a verified device result.
Delivery uses the existing main branch. A WeChat public release remains a separate publication step.

No server schema or API changes are expected; sharing uses existing database-backed public content. No new share analytics or marketing attribution is introduced.
