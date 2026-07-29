import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import postcss from "postcss";

import {
  createWechatCustomPropertiesPlugin,
  extractCustomProperties,
} from "./postcss-wechat-custom-properties.mjs";

const sharedTheme = `
:root {
  --nx-brand-900: #202A37;
  --nx-surface: #FFFFFF;
  --nx-card: var(--nx-surface);
  --nx-home-glow: rgba(223, 188, 127, .20);
}
`;

assert.deepEqual(extractCustomProperties(sharedTheme), {
  "--nx-brand-900": "#202A37",
  "--nx-surface": "#FFFFFF",
  "--nx-card": "var(--nx-surface)",
  "--nx-home-glow": "rgba(223, 188, 127, .20)",
});

assert.deepEqual(
  extractCustomProperties(`
    /* --nx-brand-900: #BADBAD; */
    :root { --nx-brand-900: #202A37; }
  `),
  { "--nx-brand-900": "#202A37" },
  "commented declarations must not override real theme tokens",
);

assert.throws(
  () => extractCustomProperties(`
    :root { --nx-tone: #111111; --nx-tone: #222222; }
  `),
  /Multiple WeChat style token definitions: --nx-tone/,
  "multiple root definitions would silently apply the last value",
);

assert.throws(
  () => extractCustomProperties(`
    @media (prefers-color-scheme: dark) {
      :root { --nx-tone: #111111; }
    }
  `),
  /Conditional WeChat style token definition: --nx-tone/,
  "conditional token definitions cannot be flattened safely",
);

const source = `
.home {
  color: var(--nx-surface);
  background: linear-gradient(135deg, var(--nx-brand-900), var(--nx-home-glow));
  padding-bottom: calc(24rpx + var(--window-bottom, 0px));
}

.card {
  background: var(--nx-card);
}
`;

const result = await postcss([
  createWechatCustomPropertiesPlugin({
    globalTokens: extractCustomProperties(sharedTheme),
  }),
]).process(source, { from: undefined });

assert.doesNotMatch(
  result.css,
  /var\(--nx-/,
  "WeChat WXSS output should inline all nx design tokens",
);
assert.doesNotMatch(
  result.css,
  /--nx-[\w-]+\s*:/,
  "compiled WXSS should not keep custom-property declarations that the renderer ignores",
);
assert.match(result.css, /color:\s*#FFFFFF/);
assert.match(
  result.css,
  /background:\s*linear-gradient\(135deg, #202A37, rgba\(223, 188, 127, \.20\)\)/,
);
assert.match(result.css, /padding-bottom:\s*calc\(24rpx \+ 0px\)/);
assert.match(result.css, /background:\s*#FFFFFF/);

const syntaxResult = await postcss([
  createWechatCustomPropertiesPlugin({
    globalTokens: extractCustomProperties(sharedTheme),
  }),
]).process(`
  .syntax {
    color: var(--nx-missing, rgb(10, 20, 30));
    content: "var(--nx-brand-900)";
    background-image: url("data:image/svg+xml,var(--nx-brand-900)");
  }
`, { from: undefined });

assert.match(
  syntaxResult.css,
  /color:\s*rgb\(10, 20, 30\)/,
  "function fallbacks should be parsed and inlined",
);
assert.match(
  syntaxResult.css,
  /content:\s*"var\(--nx-brand-900\)"/,
  "var-like text inside CSS strings is data, not a custom-property reference",
);
assert.match(
  syntaxResult.css,
  /url\("data:image\/svg\+xml,var\(--nx-brand-900\)"\)/,
  "var-like text inside quoted data URLs must remain untouched",
);

await assert.rejects(
  () => postcss([
    createWechatCustomPropertiesPlugin({ globalTokens: {} }),
  ]).process(":root { --nx-tone: red; --nx-tone: blue; }", { from: undefined }),
  /Multiple WeChat style token definitions: --nx-tone/,
  "duplicate token definitions must fail instead of silently applying the last value",
);

await assert.rejects(
  () => postcss([
    createWechatCustomPropertiesPlugin({ globalTokens: {} }),
  ]).process(`
    .a { --nx-tone: red; color: var(--nx-tone); }
    .b { color: var(--nx-tone); }
  `, { from: undefined }),
  /WeChat style tokens must be declared in :root: --nx-tone/,
  "selector-scoped declarations cannot be flattened into stylesheet-wide constants",
);

let dynamicTone = "#111111";
let tokenLoads = 0;
const dependencyFile = "/tmp/apple-mobile.css";
const dynamicPlugin = createWechatCustomPropertiesPlugin({
  globalTokens() {
    tokenLoads += 1;
    return { "--nx-dynamic": dynamicTone };
  },
  dependencyFiles: [dependencyFile],
});
const firstDynamic = await postcss([dynamicPlugin]).process(
  ".dynamic { color: var(--nx-dynamic); }",
  { from: "/tmp/first.css" },
);
dynamicTone = "#222222";
const secondDynamic = await postcss([dynamicPlugin]).process(
  ".dynamic { color: var(--nx-dynamic); }",
  { from: "/tmp/second.css" },
);
assert.match(firstDynamic.css, /#111111/);
assert.match(secondDynamic.css, /#222222/);
assert.equal(tokenLoads, 2, "watch transforms must reload the latest shared tokens");
assert.ok(
  firstDynamic.messages.some(
    (message) => message.type === "dependency" && message.file === dependencyFile,
  ),
  "each transformed stylesheet should declare the shared theme as a watch dependency",
);

for (const relativePath of [
  "../src/pages/index/index.vue",
  "../src/pages/booking/booking.vue",
  "../src/pages/test/test.vue",
]) {
  const componentSource = await readFile(new URL(relativePath, import.meta.url), "utf8");
  const scopedStyle = componentSource.match(/<style scoped>([\s\S]*?)<\/style>/)?.[1] || "";
  assert.doesNotMatch(
    scopedStyle,
    /--[\w-]+\s*:/,
    `${relativePath} should consume root tokens instead of declaring selector-scoped variables`,
  );
}

await assert.rejects(
  () => postcss([
    createWechatCustomPropertiesPlugin({ globalTokens: {} }),
  ]).process(".broken { color: var(--nx-missing); }", { from: undefined }),
  /Unresolved WeChat style token: --nx-missing/,
  "missing nx tokens should fail the build instead of silently stripping the visual style",
);

console.log("WeChat style compatibility contract: PASS");
