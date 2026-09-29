# Teacher studio UI implementation

**Goal:** Make the existing miniapp a complete teacher introduction, daily video and course registration experience, with a reviewable local preview.

**Architecture:** Keep Vue 3 / uni-app, existing API contracts, authentication, payments and booking storage. Use verified teacher copy and existing media. Gate fictional schedules, prices and local booking records behind an explicit development-only UI preview mode; never send preview writes to the live service.

**Design:** Warm ivory surfaces, copper accents, dark ink, Chinese serif feature headings, quiet borders, native four-tab navigation. Home introduces the teacher, daily videos and learning opportunities; teacher detail tells the full story; daily view filters published videos; booking supports courses, consultation and enterprise learning; profile surfaces records.

- [x] Root: shared content, design tokens/icons, homepage, teacher detail, preview isolation and app navigation.
- [x] Agent: daily video library and profile, preserving session guards.
- [x] Agent: booking form and course detail, preserving draft/intent/submit logic.
- [x] Agent: video detail presentation, preserving playback/access/payment/progress behavior.
- [x] Verify request/preview isolation, booking validation and retained core tests; compile H5 and WeChat targets.
- [x] Inspect mobile pages and key interactions in the browser; provide local preview and screenshots.

Real sources: existing `website-react/src/pages/Teacher.jsx`, local teacher/video media, public API snapshots in `miniapp/preview-data/`. Course dates/fees in the preview are illustrative. No invented testimonials, enrollment counts or scarcity.
