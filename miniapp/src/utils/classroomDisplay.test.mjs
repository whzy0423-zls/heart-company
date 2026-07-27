import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const dir = await mkdtemp(join(tmpdir(), "nx-classroom-display-"));
try {
  const source = await readFile(new URL("./classroomDisplay.js", import.meta.url), "utf8");
  const modulePath = join(dir, "classroomDisplay.mjs");
  await writeFile(modulePath, source);
  const {
    classroomAccessLabel,
    classroomContentRoute,
    classroomPurchaseAction,
    normalizeClassroomContent,
    normalizeClassroomSeries,
  } = await import(`file://${modulePath}`);

  const contractFixture = JSON.parse(
    await readFile(
      new URL("../../../docs/superpowers/fixtures/classroom-public-response.json", import.meta.url),
      "utf8",
    ),
  );
  const fixtureContent = normalizeClassroomContent(contractFixture.content);
  assert.equal(fixtureContent.id, "21");
  assert.equal(fixtureContent.title, "第一课：认识三中心");
  assert.equal(fixtureContent.contentType, "video");
  assert.equal(fixtureContent.accessLevel, "inherit");
  assert.equal(
    "url" in fixtureContent,
    false,
    "public metadata fixture must not normalize a playback URL",
  );
  assert.equal(
    "objectKey" in fixtureContent,
    false,
    "public metadata fixture must not expose an object key",
  );
  const fixtureSeries = normalizeClassroomSeries(contractFixture.series.items[0]);
  assert.equal(fixtureSeries.id, "12");
  assert.equal(fixtureSeries.title, "九型人格入门");

  const content = normalizeClassroomContent({
    id: 21,
    seriesId: 12,
    title: " 声音练习 ",
    description: " 复盘 ",
    teacherName: " 韩老师 ",
    coverUrl: "https://img.example/cover.jpg",
    contentType: "audio",
    durationSeconds: 125,
    accessLevel: "paid",
    effectiveAccess: "paid",
    priceCents: 990,
    canPlay: false,
    purchaseState: "purchase_required",
    playbackBlocked: false,
    objectKey: "private/secret.m4a",
    mediaUrl: "https://oss.example/permanent.m4a",
    ticket: "secret",
  });
  assert.deepEqual(content, {
    id: "21",
    seriesId: "12",
    title: "声音练习",
    description: "复盘",
    teacherName: "韩老师",
    coverUrl: "https://img.example/cover.jpg",
    contentType: "audio",
    durationSeconds: 125,
    accessLevel: "paid",
    effectiveAccess: "paid",
    priceCents: 990,
    canPlay: false,
    purchaseState: "purchase_required",
    playbackBlocked: false,
  });
  assert.equal("objectKey" in content, false);
  assert.equal("mediaUrl" in content, false);

  const series = normalizeClassroomSeries({
    id: 12,
    title: " 入门系列 ",
    effectiveAccess: "member",
    canPlay: true,
  });
  assert.equal(series.id, "12");
  assert.equal(series.title, "入门系列");
  assert.equal(series.effectiveAccess, "member");

  assert.equal(classroomAccessLabel("public"), "免费");
  assert.equal(classroomAccessLabel("login"), "登录可学");
  assert.equal(classroomAccessLabel("member"), "会员专享");
  assert.equal(classroomAccessLabel("paid"), "付费课件");
  assert.equal(classroomAccessLabel("unexpected"), "权限待确认");

  assert.deepEqual(classroomPurchaseAction({ canPlay: true, purchaseState: "owned" }), {
    type: "play",
    label: "继续学习",
  });
  assert.deepEqual(
    classroomPurchaseAction({
      canPlay: false,
      purchaseState: "purchase_required",
      priceCents: 990,
    }),
    { type: "purchase", label: "¥9.90 购买" },
  );
  assert.deepEqual(classroomPurchaseAction({ canPlay: false, effectiveAccess: "login" }), {
    type: "login",
    label: "登录后学习",
  });
  assert.deepEqual(classroomPurchaseAction({ canPlay: false, effectiveAccess: "member" }), {
    type: "member",
    label: "开通会员",
  });
  assert.deepEqual(classroomPurchaseAction({ playbackBlocked: true }), {
    type: "blocked",
    label: "暂不可播放",
  });

  assert.equal(
    classroomContentRoute({ id: 21, contentType: "video" }),
    "/pages/classroom-detail/classroom-detail?id=21&type=video",
  );
  assert.equal(
    classroomContentRoute({ id: 22, contentType: "audio" }),
    "/pages/classroom-detail/classroom-detail?id=22&type=audio",
  );
  assert.equal(classroomContentRoute({ id: 0, contentType: "audio" }), "");

  console.log("classroom display tests passed");
} finally {
  await rm(dir, { force: true, recursive: true });
}
