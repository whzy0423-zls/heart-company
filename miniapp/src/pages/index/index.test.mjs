import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const source = await readFile(new URL("./index.vue", import.meta.url), "utf8");

assert.ok(source, "home page should exist");
assert.match(
  source,
  /function\s+goClassroom\s*\(/,
  "home page should expose a direct teacher classroom route helper",
);
assert.match(
  source,
  /\/pages\/classroom\/classroom\?tab=standalone/,
  "home classroom entry should open standalone courseware first so published videos are visible",
);
assert.match(
  source,
  /class="[^"]*\bclassroom-spotlight\b[^"]*"/,
  "home should render a prominent classroom spotlight below the hero",
);
assert.match(
  source,
  /@click="goClassroom"/,
  "home classroom spotlight should be directly tappable",
);
assert.match(
  source,
  /class="classroom-spotlight__media"/,
  "home classroom spotlight should include a media-first visual area",
);
assert.match(
  source,
  /class="[^"]*\bclassroom-spotlight__cta\b[^"]*"/,
  "home classroom spotlight should include a clear CTA",
);
assert.match(
  source,
  /\.classroom-spotlight__cta\s*\{[^}]*min-height:\s*88rpx/s,
  "home classroom CTA should keep the miniapp minimum touch target",
);
assert.match(
  source,
  />\s*进入老师课堂\s*</,
  "home classroom CTA should use the approved entry copy",
);

console.log("home classroom spotlight tests passed");
