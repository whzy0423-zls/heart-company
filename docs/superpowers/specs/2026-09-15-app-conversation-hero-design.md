# App Conversation Hero Design

## Goal

Replace the static App screenshot in the download-page hero with a polished, realistic, anonymous growth conversation, and make the current iOS availability status permanently visible in the download area.

## Visual Direction

The phone remains the first-viewport product signal. Its screen becomes a compact conversation UI that follows the native App language: quiet warm surface, asymmetric user and AI bubbles, brand avatar, a small online state, an insight callout, and a composer. Motion is limited to message entrance, a live-status pulse, and a restrained sheen across the device; reduced-motion users receive a static composition.

The conversation uses a relatable boundary-setting scenario rather than account or login chrome:

- User: "我明明很累，却总怕拒绝别人会让关系变差。"
- AI: identifies the fear of losing recognition and separates caring for a relationship from accepting every request.
- Action: offers a sentence the user can use immediately: "我现在精力有限，这件事明天下午再回复你，可以吗？"

The UI labels the content as an anonymous example so it feels authentic without implying that private customer messages are being exposed.

## Components

- `AppConversationPreview.jsx`: semantic, static conversation markup with no API dependency and no personal data.
- `AppDownload.jsx`: replaces the hero image with the new component and retains the existing phone frame.
- `AppDownloadSection.jsx`: renders a persistent iOS availability note underneath the Android action area for all device classes.
- `index.css`: styles the phone conversation, responsive states, motion, reduced-motion behavior, and iOS notice.

## Responsive Behavior

Desktop and tablet show the full phone. The phone uses the existing fixed aspect ratio and must not crop content or create horizontal overflow. At the existing mobile breakpoint the phone remains hidden so the download section is still visible at the bottom of the first viewport.

## Accessibility

The preview is labelled as an example conversation, decorative status dots are hidden from assistive technology, contrast stays above the existing small-text baseline, and animation is disabled under `prefers-reduced-motion`. The iOS message is ordinary readable text, not color-only status.

## Verification

- Source-level regression tests verify conversation structure, copy, iOS message, and reduced-motion coverage.
- Full website tests and production build must pass.
- Browser screenshots and DOM measurements verify desktop rendering, mobile overflow, and production deployment.

