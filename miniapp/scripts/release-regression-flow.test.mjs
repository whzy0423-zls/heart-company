import assert from "node:assert/strict";
import { createRenderer } from "vue";

import TestPage from "../src/pages/test/test.vue";
import ResultPage from "../src/pages/result/result.vue";
import ProfileEditPage from "../src/pages/profile-edit/profile-edit.vue";
import BookingPage from "../src/pages/booking/booking.vue";
import RelationPage from "../src/pages/relation/relation.vue";
import { QUESTIONS } from "../src/data/enneagramGame.js";
import { calcType } from "../src/utils/enneagram.js";
import { getLastResult, setLastResult } from "../src/utils/session.js";
import { BOOKING_DRAFT_KEY } from "../src/utils/bookingDraft.js";
import {
  reportContentApi,
  reportGameResultApi,
  reportStatusApi,
  saveTestRecordApi,
} from "../src/api/index.js";

const storage = new Map([["nx_token", "miniapp-token"]]);
const hooks = {
  onHide: [],
  onLoad: [],
  onShareAppMessage: [],
  onShareTimeline: [],
  onShow: [],
  onUnload: [],
};
const state = {
  bookings: [],
  gameReports: [],
  profileUpdates: [],
  redirects: [],
  reportContentRequests: [],
  reportStatusRequests: [],
  savedRecords: [],
  toasts: [],
};
const snapshot = (value) => JSON.parse(JSON.stringify(value));
globalThis.__releaseUniHooks = hooks;

globalThis.uni = {
  getStorageSync: (key) => storage.get(key),
  setStorageSync: (key, value) => storage.set(key, snapshot(value)),
  removeStorageSync: (key) => storage.delete(key),
  redirectTo: (value) => state.redirects.push(value),
  navigateTo: () => {},
  switchTab: () => {},
  showToast: (value) => state.toasts.push(value),
  request(options) {
    const url = new URL(options.url);
    const respond = (data, statusCode = 200) =>
      queueMicrotask(() =>
        options.success({ statusCode, data: { code: statusCode < 300 ? 0 : 1, data } }),
      );
    if (options.method === "POST" && url.pathname.endsWith("/public/game-results")) {
      state.gameReports.push(snapshot(options.data));
      respond({ accepted: true });
      return;
    }
    if (options.method === "POST" && url.pathname.endsWith("/miniapp/test-records")) {
      const record = { id: "record-77", ...snapshot(options.data) };
      state.savedRecords.push(record);
      respond(record);
      return;
    }
    if (options.method === "GET" && url.pathname.endsWith("/miniapp/report/status")) {
      const id = url.searchParams.get("testRecordId");
      state.reportStatusRequests.push(id);
      respond({ unlocked: state.savedRecords.some((item) => item.id === id), priceCents: 1990 });
      return;
    }
    if (options.method === "GET" && url.pathname.endsWith("/miniapp/report/content")) {
      const id = url.searchParams.get("testRecordId");
      state.reportContentRequests.push(id);
      respond({ answer: id === "record-77" ? "纵向报告正文" : "" });
      return;
    }
    if (options.method === "GET" && url.pathname.endsWith("/wx/userinfo")) {
      respond({ id: "user-7", nickname: "旧昵称", avatar: "" });
      return;
    }
    if (options.method === "PUT" && url.pathname.endsWith("/wx/userinfo")) {
      state.profileUpdates.push(snapshot(options.data));
      respond({ id: "user-7", ...snapshot(options.data) });
      return;
    }
    if (options.method === "POST" && url.pathname.endsWith("/miniapp/bookings")) {
      state.bookings.push(snapshot(options.data));
      respond({ id: "booking-9", ...snapshot(options.data) });
      return;
    }
    queueMicrotask(() =>
      options.fail({ errMsg: `unhandled release request ${options.method} ${url.pathname}` }),
    );
  },
};

const renderer = createRenderer({
  createComment: (text) => ({ text, type: "comment" }),
  createElement: (type) => ({ children: [], props: {}, type }),
  createText: (text) => ({ text, type: "text" }),
  insert(child, parent) {
    parent.children ||= [];
    parent.children.push(child);
    child.parent = parent;
  },
  nextSibling: () => null,
  parentNode: (node) => node.parent || null,
  patchProp(node, key, _previous, value) {
    node.props[key] = value;
  },
  remove(node) {
    if (node.parent) node.parent.children = node.parent.children.filter((item) => item !== node);
  },
  setElementText(node, text) {
    node.text = text;
  },
  setText(node, text) {
    node.text = text;
  },
});

function mount(component) {
  const root = { children: [], type: "root" };
  const app = renderer.createApp(component);
  app.mount(root);
  return { app, state: app._instance.setupState };
}

function resetHooks() {
  for (const callbacks of Object.values(hooks)) callbacks.length = 0;
}

async function settle(predicate, label) {
  for (let attempt = 0; attempt < 50; attempt += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
    if (predicate()) return;
  }
  assert.fail(`timed out waiting for ${label}`);
}

try {
  resetHooks();
  const testPage = mount(TestPage);
  assert.equal(
    testPage.state.setLastResult,
    setLastResult,
    "test.vue must import the production session writer",
  );
  assert.equal(
    testPage.state.reportGameResultApi,
    reportGameResultApi,
    "test.vue must import the production API wrapper",
  );
  testPage.state.start("female");
  testPage.state.answers = QUESTIONS.map((question) => question.options[0]);
  const expectedResult = calcType(testPage.state.answers, "female");
  testPage.state.finish();
  await settle(() => state.gameReports.length === 1, "anonymous result report");

  const cached = storage.get("nx_last_test_result");
  assert.deepEqual(
    Object.keys(cached).sort(),
    ["gender", "result"],
    "session envelope schema changed",
  );
  assert.deepEqual(
    Object.keys(cached.result).sort(),
    ["centers", "score", "second", "type"],
    "session result schema changed",
  );
  assert.deepEqual(cached, { gender: "female", result: expectedResult });
  assert.deepEqual(
    getLastResult(),
    cached,
    "result page session reader must consume the test page write",
  );
  assert.deepEqual(state.gameReports[0], {
    gender: "female",
    resultType: expectedResult.type,
    secondType: expectedResult.second || 0,
    score: expectedResult.score,
    centers: expectedResult.centers,
  });
  assert.deepEqual(state.redirects, [{ url: "/pages/result/result" }]);
  testPage.app.unmount();

  resetHooks();
  const resultPage = mount(ResultPage);
  assert.equal(
    resultPage.state.getLastResult,
    getLastResult,
    "result.vue must import the production session reader",
  );
  assert.equal(
    resultPage.state.saveTestRecordApi,
    saveTestRecordApi,
    "result.vue must import the production save API",
  );
  assert.equal(
    resultPage.state.reportStatusApi,
    reportStatusApi,
    "result.vue must import the production status API",
  );
  assert.equal(
    resultPage.state.reportContentApi,
    reportContentApi,
    "result.vue must import the production report API",
  );
  assert.deepEqual(
    resultPage.state.result,
    expectedResult,
    "result.vue did not consume test.vue's stored result",
  );
  await resultPage.state.saveRecord();
  await settle(
    () => resultPage.state.reportContent === "纵向报告正文",
    "report content DTO consumption",
  );
  assert.deepEqual(state.savedRecords[0], {
    id: "record-77",
    gender: "female",
    resultType: expectedResult.type,
    secondType: expectedResult.second || 0,
    score: expectedResult.score,
    centers: expectedResult.centers,
  });
  assert.deepEqual(state.reportStatusRequests, ["record-77"], "report status query DTO changed");
  assert.deepEqual(state.reportContentRequests, ["record-77"], "report content query DTO changed");
  assert.equal(resultPage.state.recordId, "record-77");
  assert.equal(resultPage.state.saved, true);
  assert.equal(resultPage.state.reportUnlocked, true);
  assert.equal(resultPage.state.reportContent, "纵向报告正文");
  resultPage.app.unmount();

  storage.set(BOOKING_DRAFT_KEY, {
    ts: 1,
    data: {
      kind: "course",
      contactName: "草稿用户",
      phone: "13812345678",
      intent: "课程",
      preferredTime: "",
      message: "",
    },
  });
  resetHooks();
  const bookingPage = mount(BookingPage);
  assert.equal(bookingPage.state.restoredDraftNotice, true);
  await bookingPage.state.submit();
  assert.equal(state.bookings[0].kind, "course");
  assert.equal(state.bookings[0].contactName, "草稿用户");
  assert.equal(storage.has(BOOKING_DRAFT_KEY), false);
  assert.equal(bookingPage.state.form.contactName, "");
  bookingPage.app.unmount();

  resetHooks();
  const profilePage = mount(ProfileEditPage);
  assert.equal(hooks.onShow.length, 1);
  hooks.onShow[0]();
  await settle(() => profilePage.state.user?.nickname === "旧昵称", "profile load");
  profilePage.state.nicknameDraft = " 新昵称 ";
  profilePage.state.avatarDraft = "https://avatar.example/new.png";
  await profilePage.state.saveProfile();
  assert.deepEqual(state.profileUpdates, [
    { nickname: "新昵称", avatar: "https://avatar.example/new.png" },
  ]);
  assert.equal(profilePage.state.user.nickname, "新昵称");
  profilePage.app.unmount();

  resetHooks();
  const relationPage = mount(RelationPage);
  assert.equal(hooks.onLoad.length, 1);
  hooks.onLoad[0]({ type: "1" });
  relationPage.state.pickTa(2);
  relationPage.state.analyze();
  assert.equal(relationPage.state.stage, "result");
  assert.equal(relationPage.state.analysis.score, 82);
  relationPage.state.reset();
  assert.equal(relationPage.state.stage, "pick");
  relationPage.app.unmount();

  console.log("release production SFC regression flow tests passed");
} finally {
  delete globalThis.__releaseUniHooks;
  delete globalThis.uni;
}
