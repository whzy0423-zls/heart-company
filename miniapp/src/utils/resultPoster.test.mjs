import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const dir = await mkdtemp(join(tmpdir(), "nx-miniapp-result-poster-"));
const modulePath = join(dir, "resultPoster.mjs");
await writeFile(modulePath, await readFile(new URL("./resultPoster.js", import.meta.url), "utf8"));

const { createResultPoster, wrapCanvasText } = await import(`file://${modulePath}`);

const lines = [];
const ctx = {
  font: "14px sans-serif",
  measureText(value) {
    return { width: String(value).length * 10 };
  },
  fillText(value, x, y) {
    lines.push({ value, x, y });
  },
};
wrapCanvasText(ctx, "一二三四五六", 10, 20, 35, 12);
assert.deepEqual(
  lines.map((line) => line.value),
  ["一二三", "四五六"],
  "poster text should wrap without empty lines",
);

const baseCanvas = {
  width: 0,
  height: 0,
  getContext() {
    return {
      ...ctx,
      scale() {},
      save() {},
      restore() {},
      beginPath() {},
      arc() {},
      clip() {},
      drawImage() {},
      fillRect() {},
      stroke() {},
    };
  },
  createImage() {
    const image = {};
    Object.defineProperty(image, "src", {
      set() {
        queueMicrotask(() => image.onload());
      },
    });
    return image;
  },
};

const runtime = {
  createSelectorQuery() {
    return {
      select() {
        return {
          fields() {
            return this;
          },
          exec(callback) {
            callback([{ node: baseCanvas }]);
          },
        };
      },
    };
  },
  getSystemInfoSync() {
    return { pixelRatio: 2 };
  },
  canvasToTempFilePath(options) {
    options.success({ tempFilePath: "/tmp/poster.png" });
  },
};

const poster = await createResultPoster({
  instance: {},
  result: { type: 4 },
  info: { color: "blue", en: "The Individualist", keywords: "理想主义" },
  summary: "一段用于海报的摘要",
  title: "艺术型",
  runtime,
});
assert.equal(poster, "/tmp/poster.png", "poster generation should resolve the exported temp path");

for (const [name, canvasToTempFilePath, message] of [
  [
    "export callback failure",
    (options) => options.fail(new Error("canvas export failed")),
    /canvas export failed/,
  ],
  ["empty export path", (options) => options.success({}), /海报导出路径缺失/],
]) {
  await assert.rejects(
    () =>
      createResultPoster({
        instance: {},
        result: { type: 4 },
        info: { color: "blue", en: "", keywords: "" },
        summary: "",
        title: "",
        runtime: { ...runtime, canvasToTempFilePath },
      }),
    message,
    `${name} should reject so the page can offer retry`,
  );
}

const avatarFailureCanvas = {
  ...baseCanvas,
  createImage() {
    const image = {};
    Object.defineProperty(image, "src", {
      set() {
        queueMicrotask(() => image.onerror());
      },
    });
    return image;
  },
};
await assert.rejects(
  () =>
    createResultPoster({
      instance: {},
      result: { type: 4 },
      info: { color: "blue", en: "", keywords: "" },
      summary: "",
      title: "",
      runtime: {
        ...runtime,
        createSelectorQuery() {
          return {
            select() {
              return {
                fields() {
                  return this;
                },
                exec(callback) {
                  callback([{ node: avatarFailureCanvas }]);
                },
              };
            },
          };
        },
      },
    }),
  /海报头像加载失败/,
  "avatar load failure should reject so the page can offer retry",
);

await assert.rejects(
  () =>
    createResultPoster({
      instance: {},
      result: { type: 4 },
      info: {},
      summary: "",
      title: "",
      runtime: {
        createSelectorQuery() {
          return {
            select() {
              return {
                fields() {
                  return this;
                },
                exec(callback) {
                  callback([{}]);
                },
              };
            },
          };
        },
      },
    }),
  /海报画布未找到/,
  "missing canvas should reject with a stable error",
);

console.log("result poster utility tests passed");
await rm(dir, { force: true, recursive: true });
