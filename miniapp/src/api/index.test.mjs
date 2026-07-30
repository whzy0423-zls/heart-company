import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const source = readFileSync(new URL("./index.js", import.meta.url), "utf8");

assert.match(
  source,
  /url:\s*['"]\/app\/auth\/sms\/send['"]/,
  "sendAppSmsApi must call /app/auth/sms/send under the /api BaseURL",
);
assert.match(
  source,
  /url:\s*['"]\/app\/auth\/sms\/login['"]/,
  "loginByAppSmsApi must call /app/auth/sms/login under the /api BaseURL",
);
assert.match(
  source,
  /url:\s*['"]\/app\/push\/register['"]/,
  "registerAppPushApi must call /app/push/register under the /api BaseURL",
);
assert.match(
  source,
  /url:\s*['"]\/app\/push\/unregister['"]/,
  "unregisterAppPushApi must call /app/push/unregister under the /api BaseURL",
);
assert.doesNotMatch(
  source,
  /url:\s*['"]\/api\/app\/auth/,
  "API paths must not include /api because API_BASE already ends with /api",
);

const dir = await mkdtemp(join(tmpdir(), "nx-miniapp-classroom-api-"));
try {
  const executableSource = source
    .replace(
      /import\s+\{[^}]+\}\s+from\s+['"]\.\/request['"]/,
      "import { clearToken, request, getToken } from './request.mjs'",
    )
    .replace(
      /import\s+\{\s*APP_CHANNEL\s*\}\s+from\s+['"]\.\.\/config['"]/,
      "const APP_CHANNEL = 'test'",
    )
    .replace(
      /import\s+\{\s*userErrorMessage\s*\}\s+from\s+['"]\.\.\/utils\/userMessage['"]/,
      "import { userErrorMessage } from './userMessage.mjs'",
    );
  await writeFile(join(dir, "index.mjs"), executableSource);
  await writeFile(
    join(dir, "userMessage.mjs"),
    readFileSync(new URL("../utils/userMessage.js", import.meta.url), "utf8"),
  );
  await writeFile(
    join(dir, "request.mjs"),
    `
    export const calls = []
    export let token = ''
    export let responder = async (options) => ({ options })
    export function getToken() { return token }
    export function setTestToken(value) { token = value }
    export function clearToken() { token = '' }
    export function setResponder(value) { responder = value }
    export function request(options) { calls.push(options); return responder(options) }
  `,
  );

  const api = await import(`file://${join(dir, "index.mjs")}`);
  const requestStub = await import(`file://${join(dir, "request.mjs")}`);
  const fixture = JSON.parse(
    readFileSync(
      new URL("../../../docs/superpowers/fixtures/classroom-public-response.json", import.meta.url),
      "utf8",
    ),
  );

  requestStub.setTestToken("");
  await api.listClassroomSeriesApi({ limit: 10, offset: 20, contentType: "video" });
  await api.listClassroomStandaloneApi({ limit: 5 });
  await api.listClassroomRecentApi({ limit: 2 });
  await api.getClassroomSeriesApi("12");
  await api.getClassroomContentApi(21);
  assert.deepEqual(requestStub.calls.slice(-5), [
    {
      url: "/public/classroom/series",
      method: "GET",
      query: { limit: 10, offset: 20, contentType: "video" },
      auth: false,
    },
    { url: "/public/classroom/standalone", method: "GET", query: { limit: 5 }, auth: false },
    { url: "/public/classroom/recent", method: "GET", query: { limit: 2 }, auth: false },
    { url: "/public/classroom/series/12", method: "GET", auth: false },
    { url: "/public/classroom/content/21", method: "GET", auth: false },
  ]);

  requestStub.setTestToken("jwt");
  await api.getClassroomContentApi(21);
  assert.equal(
    requestStub.calls.at(-1).auth,
    true,
    "metadata requests should use JWT when one is present",
  );

  requestStub.setTestToken("");
  requestStub.setResponder(async (options) => {
    if (options.url.endsWith("/ticket")) return { ticket: "anon-ticket", expiresIn: 300 };
    return { url: "https://signed.example/lesson.mp4", expiresIn: 300, contentType: "video" };
  });
  const anonymousPlayback = await api.getClassroomPlaybackApi(21);
  assert.equal(anonymousPlayback.url, "https://signed.example/lesson.mp4");
  assert.deepEqual(requestStub.calls.slice(-2), [
    { url: "/public/classroom/content/21/ticket", method: "POST", data: {} },
    {
      url: "/miniapp/classroom/content/21/play",
      method: "POST",
      data: { ticket: "anon-ticket" },
      auth: false,
    },
  ]);

  requestStub.setTestToken("jwt");
  await api.getClassroomPlaybackApi(22);
  assert.deepEqual(requestStub.calls.at(-1), {
    url: "/miniapp/classroom/content/22/play",
    method: "POST",
    data: {},
    auth: true,
  });

  await api.createClassroomOrderApi("series", 12);
  await api.getClassroomOrderStatusApi("content", 21);
  await api.devPayClassroomOrderApi("cls-21");
  await api.updateClassroomProgressApi(21, 91);
  await api.getClassroomContinueLearningApi();
  assert.deepEqual(requestStub.calls.slice(-5), [
    {
      url: "/miniapp/classroom/orders",
      method: "POST",
      data: { targetType: "series", refId: "12" },
      auth: true,
    },
    {
      url: "/miniapp/classroom/orders/status",
      method: "GET",
      query: { targetType: "content", refId: "21" },
      auth: true,
    },
    {
      url: "/miniapp/classroom/orders/dev-pay",
      method: "POST",
      data: { outTradeNo: "cls-21" },
      auth: true,
    },
    {
      url: "/miniapp/classroom/content/21/progress",
      method: "PUT",
      data: { positionSeconds: 91 },
      auth: true,
    },
    { url: "/miniapp/classroom/continue-learning", method: "GET", auth: true },
  ]);

  let playbackVersion = 0;
  requestStub.setResponder(async (options) => {
    if (options.url.endsWith("/play"))
      return {
        url: `https://signed.example/v${++playbackVersion}`,
        expiresIn: 300,
        contentType: "audio",
      };
    return {};
  });
  const consumed = await api.withClassroomPlaybackRetry(21, async (playback) => {
    if (playbackVersion === 1)
      throw Object.assign(new Error("ExpiredToken"), { statusCode: 403, code: "ExpiredToken" });
    return playback.url;
  });
  assert.equal(
    consumed,
    "https://signed.example/v2",
    "expired signed URLs should refresh exactly once",
  );

  requestStub.setResponder(async () => {
    throw new Error(" 后端错误 ");
  });
  await assert.rejects(() => api.getClassroomContentApi(21), { message: "后端错误" });

  requestStub.setTestToken("stale-jwt");
  let staleMetadataCalls = 0;
  requestStub.setResponder(async (options) => {
    staleMetadataCalls += 1;
    if (options.auth)
      throw Object.assign(new Error("Unauthorized"), { statusCode: 401, authExpired: true });
    return { id: 21, effectiveAccess: "public", canPlay: true };
  });
  const publicAfterStaleJWT = await api.getClassroomContentApi(21);
  assert.equal(publicAfterStaleJWT.canPlay, true);
  assert.equal(
    staleMetadataCalls,
    2,
    "optional-auth metadata should retry anonymously exactly once",
  );
  assert.equal(requestStub.token, "", "stale classroom JWT should clear the local session");
  assert.deepEqual(
    requestStub.calls.slice(-2).map(({ auth }) => auth),
    [true, false],
  );

  requestStub.setTestToken("corrupt-jwt");
  let protectedMetadataCalls = 0;
  requestStub.setResponder(async (options) => {
    protectedMetadataCalls += 1;
    if (options.auth) throw Object.assign(new Error("Forbidden"), { statusCode: 403 });
    return { id: 22, effectiveAccess: "login", canPlay: false, purchaseState: "available" };
  });
  const protectedAfterFallback = await api.getClassroomContentApi(22);
  assert.equal(
    protectedAfterFallback.canPlay,
    false,
    "anonymous fallback must preserve server access decisions",
  );
  assert.equal(protectedMetadataCalls, 2);
  assert.equal(requestStub.token, "");

  requestStub.setTestToken("jwt");
  let nonAuthFailureCalls = 0;
  requestStub.setResponder(async () => {
    nonAuthFailureCalls += 1;
    throw Object.assign(new Error("Server Error"), { statusCode: 500 });
  });
  await assert.rejects(() => api.listClassroomSeriesApi(), { message: "Server Error" });
  assert.equal(nonAuthFailureCalls, 1, "non-auth errors must not trigger anonymous retry");

  requestStub.setTestToken("jwt");
  let repeatedAuthFailures = 0;
  requestStub.setResponder(async () => {
    repeatedAuthFailures += 1;
    throw Object.assign(new Error("Unauthorized"), { statusCode: 401 });
  });
  await assert.rejects(() => api.listClassroomStandaloneApi(), { message: "Unauthorized" });
  assert.equal(repeatedAuthFailures, 2, "auth fallback must stop after one anonymous retry");

  requestStub.setTestToken("expired-jwt");
  const stalePlaybackCalls = [];
  requestStub.setResponder(async (options) => {
    stalePlaybackCalls.push(options);
    if (options.url.endsWith("/play") && options.auth)
      throw Object.assign(new Error("Unauthorized"), { statusCode: 401 });
    if (options.url.endsWith("/ticket"))
      return { ticket: "fresh-anonymous-ticket", expiresIn: 300 };
    return {
      url: "https://signed.example/anonymous-retry.mp4",
      expiresIn: 300,
      contentType: "video",
    };
  });
  const playbackAfterStaleJWT = await api.getClassroomPlaybackApi(23);
  assert.equal(playbackAfterStaleJWT.url, "https://signed.example/anonymous-retry.mp4");
  assert.equal(requestStub.token, "");
  assert.deepEqual(
    stalePlaybackCalls.map(({ url, auth, data }) => ({ url, auth, data })),
    [
      { url: "/miniapp/classroom/content/23/play", auth: true, data: {} },
      { url: "/public/classroom/content/23/ticket", auth: undefined, data: {} },
      {
        url: "/miniapp/classroom/content/23/play",
        auth: false,
        data: { ticket: "fresh-anonymous-ticket" },
      },
    ],
  );

  requestStub.setTestToken("expired-jwt");
  let protectedPlaybackCalls = 0;
  requestStub.setResponder(async (options) => {
    protectedPlaybackCalls += 1;
    if (options.auth) throw Object.assign(new Error("Unauthorized"), { statusCode: 401 });
    throw Object.assign(new Error("Not Found"), { statusCode: 404 });
  });
  await assert.rejects(() => api.getClassroomPlaybackApi(24), { message: "Not Found" });
  assert.equal(
    protectedPlaybackCalls,
    2,
    "protected content fallback should stop when anonymous ticket is denied",
  );

  assert.equal(fixture.series.items[0].objectKey, undefined);
  assert.equal(fixture.content.url, undefined);
  assert.equal(fixture.playback.expiresIn, 300);
} finally {
  await rm(dir, { force: true, recursive: true });
}

console.log("api index tests passed");
