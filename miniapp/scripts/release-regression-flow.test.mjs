import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const root = new URL("../src/pages/", import.meta.url);
const directory = await mkdtemp(join(tmpdir(), "nine-xing-release-flow-"));
let sequence = 0;

async function loadPage(relativePath, prelude, exports) {
  const source = await readFile(new URL(relativePath, root), "utf8");
  const script = source.match(/<script setup>([\s\S]*?)<\/script>/)?.[1];
  assert.ok(script, `${relativePath} must expose script setup`);
  const executable = script.replace(/^import[\s\S]*?from\s+['"][^'"]+['"]\s*$/gm, "");
  const modulePath = join(directory, `page-${++sequence}.mjs`);
  await writeFile(modulePath, `${prelude}\n${executable}\nexport { ${exports.join(", ")} };\n`);
  return import(`${pathToFileURL(modulePath).href}?case=${sequence}`);
}

try {
  {
    const state = { reports: [], results: [], redirects: [] };
    globalThis.__releaseTestFlow = state;
    globalThis.uni = { redirectTo: (value) => state.redirects.push(value) };
    const page = await loadPage(
      "test/test.vue",
      `
const ref = (value) => ({ value });
const computed = (getter) => ({ get value() { return getter(); } });
const onUnload = () => {};
const QUESTIONS = [{ q: 'fixture', options: [{ t: 'A' }] }];
const calcType = () => ({ type: 1, second: 2, score: { 1: 9 }, centers: [{ key: 'gut', score: 9 }] });
const setLastResult = (result, gender) => globalThis.__releaseTestFlow.results.push({ result, gender });
const reportGameResultApi = async (payload) => { globalThis.__releaseTestFlow.reports.push(payload); };
`,
      ["start", "choose"],
    );
    page.start("female");
    page.choose({ t: "A" });
    await Promise.resolve();
    assert.equal(state.results[0].gender, "female");
    assert.equal(state.reports[0].resultType, 1);
    assert.deepEqual(state.redirects, [{ url: "/pages/result/result" }]);
  }

  {
    const state = { saved: [], statuses: [], contents: [], toasts: [] };
    globalThis.__releaseResultFlow = state;
    globalThis.uni = { showToast: (value) => state.toasts.push(value) };
    const page = await loadPage(
      "result/result.vue",
      `
const ref = (value) => ({ value });
const computed = (getter) => ({ get value() { return getter(); } });
const getCurrentInstance = () => ({});
const onMounted = (handler) => handler();
const onShareAppMessage = () => {};
const onShareTimeline = () => {};
const TYPES_INFO = {
  1: { center: 'gut', growth: 2, stress: 2, name: '一号' },
  2: { center: 'heart', growth: 1, stress: 1, name: '二号' },
};
const CENTERS = { gut: { name: '本能' }, heart: { name: '情感' } };
const RESULTS = { 1: { title: '一号', summary: 'summary' } };
const isWing = () => true;
const resultPersonaText = () => 'persona';
const getLastResult = () => ({ gender: 'female', result: { type: 1, second: 2, score: { 1: 9 }, centers: [{ key: 'gut', score: 9 }] } });
const normalizeLastResult = (value) => value;
const ensureLogin = async () => {};
const saveTestRecordApi = async (payload) => { globalThis.__releaseResultFlow.saved.push(payload); return { id: 'record-77' }; };
const reportStatusApi = async (id) => { globalThis.__releaseResultFlow.statuses.push(id); return { unlocked: true }; };
const reportContentApi = async (id) => { globalThis.__releaseResultFlow.contents.push(id); return { answer: '纵向报告正文' }; };
const payForReport = async () => {};
const userErrorMessage = (error, fallback) => error?.message || fallback;
const reportDisplayState = () => ({ key: 'ready' });
const createResultPoster = async () => '';
`,
      ["saveRecord", "recordId", "saved", "reportUnlocked", "reportContent"],
    );
    await page.saveRecord();
    await new Promise((resolve) => setTimeout(resolve, 0));
    assert.equal(page.recordId.value, "record-77");
    assert.equal(page.saved.value, true);
    assert.equal(state.saved[0].resultType, 1);
    assert.deepEqual(state.statuses, ["record-77"]);
    assert.deepEqual(state.contents, ["record-77"]);
    assert.equal(page.reportUnlocked.value, true);
    assert.equal(page.reportContent.value, "纵向报告正文");
  }

  {
    const state = { token: "miniapp-token", updates: [], toasts: [], show: null };
    globalThis.__releaseProfileFlow = state;
    globalThis.uni = {
      showToast: (value) => state.toasts.push(value),
      switchTab: () => {},
    };
    const page = await loadPage(
      "profile-edit/profile-edit.vue",
      `
const ref = (value) => ({ value });
const onShow = (handler) => { globalThis.__releaseProfileFlow.show = handler; };
const onHide = () => {};
const onUnload = () => {};
const getToken = () => globalThis.__releaseProfileFlow.token;
const clearToken = () => { globalThis.__releaseProfileFlow.token = ''; };
const normalizeWechatProfile = (value) => ({ nickname: String(value.nickname || '').trim(), avatar: String(value.avatar || '').trim() });
const hasProfilePayload = (value) => Boolean(value.nickname || value.avatar);
const getWechatProfilePayload = async () => ({});
const userErrorMessage = (error, fallback) => error?.message || fallback;
const getUserInfoApi = async () => ({ nickname: '旧昵称', avatar: '' });
const updateUserInfoApi = async (payload) => { globalThis.__releaseProfileFlow.updates.push(payload); return { ...payload, id: 'user-7' }; };
`,
      ["nicknameDraft", "avatarDraft", "user", "saveProfile"],
    );
    state.show();
    await new Promise((resolve) => setTimeout(resolve, 0));
    page.nicknameDraft.value = " 新昵称 ";
    page.avatarDraft.value = "https://avatar.example/new.png";
    await page.saveProfile();
    assert.deepEqual(state.updates, [
      { nickname: "新昵称", avatar: "https://avatar.example/new.png" },
    ]);
    assert.equal(page.user.value.nickname, "新昵称");
    assert.ok(state.toasts.some((item) => item.title === "资料已保存" && item.icon === "success"));
  }

  {
    const state = { created: [], cleared: 0, toasts: [] };
    globalThis.__releaseBookingFlow = state;
    globalThis.uni = { showToast: (value) => state.toasts.push(value) };
    const page = await loadPage(
      "booking/booking.vue",
      `
const ref = (value) => ({ value });
const watch = () => {};
const onHide = () => {};
const onUnload = () => {};
const ensureLogin = async () => {};
const createBookingApi = async (payload) => { globalThis.__releaseBookingFlow.created.push(payload); return { id: 'booking-9' }; };
const userErrorMessage = (error, fallback) => error?.message || fallback;
const clearBookingDraft = () => { globalThis.__releaseBookingFlow.cleared += 1; };
const loadBookingDraft = () => ({ kind: 'course', contactName: '草稿用户', phone: '13812345678', intent: '课程' });
const saveBookingDraft = () => {};
`,
      ["form", "kindIndex", "restoredDraftNotice", "submit"],
    );
    assert.equal(page.restoredDraftNotice.value, true);
    await page.submit();
    assert.equal(state.created[0].kind, "course");
    assert.equal(state.created[0].contactName, "草稿用户");
    assert.equal(state.cleared, 1);
    assert.equal(page.form.value.contactName, "");
    assert.ok(state.toasts.some((item) => item.title === "预约已提交"));
  }

  {
    const state = { loads: [], toasts: [] };
    globalThis.__releaseRelationFlow = state;
    globalThis.uni = {
      showToast: (value) => state.toasts.push(value),
      redirectTo: () => {},
    };
    const page = await loadPage(
      "relation/relation.vue",
      `
const ref = (value) => ({ value });
const onLoad = (handler) => { globalThis.__releaseRelationFlow.loads.push(handler); };
const TYPES_INFO = {
  1: { center: 'gut', name: '一号', desire: '正确' },
  2: { center: 'heart', name: '二号', desire: '被需要' },
};
const CENTERS = { gut: { name: '本能中心' }, heart: { name: '情感中心' } };
const isValidTypeId = (value) => Number(value) === 1 || Number(value) === 2;
const normalizeTypeId = (value) => isValidTypeId(value) ? Number(value) : 0;
`,
      ["myType", "taType", "analysis", "stage", "pickMy", "pickTa", "analyze", "reset"],
    );
    state.loads[0]({ type: "1" });
    page.pickTa(2);
    page.analyze();
    assert.equal(page.stage.value, "result");
    assert.equal(page.analysis.value.score, 82);
    assert.match(page.analysis.value.myDrive, /一号/);
    page.reset();
    assert.equal(page.stage.value, "pick");
  }

  console.log("release regression flow tests passed");
} finally {
  delete globalThis.__releaseTestFlow;
  delete globalThis.__releaseResultFlow;
  delete globalThis.__releaseProfileFlow;
  delete globalThis.__releaseBookingFlow;
  delete globalThis.__releaseRelationFlow;
  delete globalThis.uni;
  await rm(directory, { force: true, recursive: true });
}
