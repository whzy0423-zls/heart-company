# Result Button Alignment Design

## Goal

Correct the visibly top-shifted labels in the result page action buttons without changing the approved layout, colors, hierarchy, or navigation behavior.

## Design

Keep the current button heights and visual treatments. Apply an explicit flex centering contract to the result report buttons and the result action buttons: zero vertical padding, `display: flex`, `align-items: center`, `justify-content: center`, and a compact line height. This avoids relying on the WeChat native `button` line box, whose default internal metrics make the labels appear too high.

## Scope

- Update only `miniapp/src/pages/result/result.vue` button alignment styles.
- Add a source-level regression check to `miniapp/scripts/ui-compat.test.mjs`.
- Rebuild the WeChat mini program and visually verify the result page.

## Success Criteria

- Report CTA, share, poster, relation, booking, and restart labels are vertically centered.
- Existing button colors, borders, widths, gaps, and click handlers remain unchanged.
- UI compatibility tests and the WeChat production build pass.
