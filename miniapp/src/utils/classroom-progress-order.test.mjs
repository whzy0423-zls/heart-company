import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const dir = await mkdtemp(join(tmpdir(), "nx-classroom-order-"));
try {
  const source = await readFile(new URL("./classroomProgress.js", import.meta.url), "utf8");
  const modulePath = join(dir, "classroomProgress.mjs");
  await writeFile(modulePath, source);
  const { CLASSROOM_ORDER_POLL_MAX_ATTEMPTS, createClassroomPurchaseController } = await import(
    `file://${modulePath}`
  );

  assert.equal(CLASSROOM_ORDER_POLL_MAX_ATTEMPTS, 6, "payment polling must be bounded");

  const states = [];
  let createCalls = 0;
  let statusCalls = 0;
  let successCalls = 0;
  const successful = createClassroomPurchaseController({
    create: async () => {
      createCalls += 1;
      return { outTradeNo: "cls-1", payParams: { package: "prepay_id=1" } };
    },
    pay: async () => {},
    status: async () => {
      statusCalls += 1;
      return statusCalls >= 2 ? { status: "paid", owned: true } : { status: "pending" };
    },
    wait: async () => {},
    onChange: (snapshot) => states.push(snapshot.state),
    onSuccess: async () => {
      successCalls += 1;
    },
  });
  await successful.purchase();
  assert.equal(createCalls, 1);
  assert.equal(statusCalls, 2);
  assert.equal(successCalls, 1);
  assert.deepEqual(states, ["creating", "pending", "pending", "success"]);

  let concurrentCreates = 0;
  let releasePayment;
  const paymentGate = new Promise((resolve) => {
    releasePayment = resolve;
  });
  const concurrent = createClassroomPurchaseController({
    create: async () => {
      concurrentCreates += 1;
      return { outTradeNo: "cls-2", payParams: {} };
    },
    pay: async () => paymentGate,
    status: async () => ({ status: "paid", owned: true }),
    wait: async () => {},
  });
  const firstPurchase = concurrent.purchase();
  const duplicatePurchase = concurrent.purchase();
  assert.equal(firstPurchase, duplicatePurchase, "duplicate taps must share one pending checkout");
  assert.equal(concurrentCreates, 1, "duplicate taps must not create another pending order");
  releasePayment();
  await firstPurchase;

  let attempts = 0;
  const boundedStates = [];
  const bounded = createClassroomPurchaseController({
    create: async () => ({ outTradeNo: "cls-3", payParams: {} }),
    pay: async () => {},
    status: async () => {
      attempts += 1;
      return { status: "pending" };
    },
    wait: async () => {},
    onChange: (snapshot) => boundedStates.push(snapshot),
  });
  await bounded.purchase();
  assert.equal(attempts, CLASSROOM_ORDER_POLL_MAX_ATTEMPTS);
  assert.equal(bounded.snapshot().state, "failure");
  assert.match(bounded.snapshot().message, /确认|重试/);
  assert.ok(boundedStates.length > 0);

  let stoppedChecks = 0;
  let releaseWait;
  const stopWait = new Promise((resolve) => {
    releaseWait = resolve;
  });
  const stopped = createClassroomPurchaseController({
    create: async () => ({ outTradeNo: "cls-4", payParams: {} }),
    pay: async () => {},
    status: async () => {
      stoppedChecks += 1;
      return { status: "pending" };
    },
    wait: async () => stopWait,
  });
  const stoppedPurchase = stopped.purchase();
  await Promise.resolve();
  await Promise.resolve();
  stopped.stop();
  releaseWait();
  await stoppedPurchase;
  assert.equal(stoppedChecks, 1, "page disposal must stop payment polling");

  let retries = 0;
  const retryStates = [];
  const retrying = createClassroomPurchaseController({
    create: async () => ({ outTradeNo: "cls-5", payParams: {} }),
    pay: async () => {
      retries += 1;
      if (retries === 1) throw { errMsg: "requestPayment:fail cancel" };
    },
    status: async () => ({ status: "paid", owned: true }),
    wait: async () => {},
    onChange: (snapshot) => retryStates.push(snapshot.state),
  });
  await retrying.purchase();
  assert.equal(retrying.snapshot().state, "cancelled");
  await retrying.retry();
  assert.equal(retrying.snapshot().state, "success");
  assert.ok(retryStates.includes("cancelled"));
  assert.ok(retryStates.includes("success"));

  console.log("classroom progress and order tests passed");
} finally {
  await rm(dir, { force: true, recursive: true });
}
