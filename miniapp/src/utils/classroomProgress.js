export const CLASSROOM_PROGRESS_THROTTLE_MS = 12_000;
export const CLASSROOM_ORDER_POLL_MAX_ATTEMPTS = 6;
const LOCAL_PREFIX = "nx_classroom_progress:";

function contentID(value) {
  const normalized = String(value ?? "").trim();
  return /^\d+$/.test(normalized) && normalized !== "0" ? normalized : "";
}

function position(value, durationSeconds = 0) {
  const normalized = Math.max(0, Math.floor(Number(value) || 0));
  const duration = Math.max(0, Math.floor(Number(durationSeconds) || 0));
  return duration > 0 ? Math.min(normalized, duration) : normalized;
}

export function classroomCompletion(positionSeconds, durationSeconds) {
  const duration = Math.max(0, Number(durationSeconds) || 0);
  if (duration <= 0) return { ratio: 0, completed: false };
  const ratio = Math.min(1, Math.max(0, Number(positionSeconds) || 0) / duration);
  return { ratio, completed: ratio >= 0.9 };
}

function localKey(id) {
  return `${LOCAL_PREFIX}${id}`;
}

export function readAnonymousClassroomProgress(storage, value) {
  const id = contentID(value);
  if (!storage || !id) return null;
  try {
    const raw = storage.getItem(localKey(id));
    const parsed = typeof raw === "string" ? JSON.parse(raw) : raw;
    if (!parsed || typeof parsed !== "object") return null;
    return {
      positionSeconds: position(parsed.positionSeconds),
      completed: parsed.completed === true,
      updatedAt: Math.max(0, Number(parsed.updatedAt) || 0),
    };
  } catch {
    return null;
  }
}

export function createClassroomProgressTracker(options = {}) {
  const id = contentID(options.contentId);
  if (!id) throw new Error("课件参数无效");
  const durationSeconds = Math.max(0, Number(options.durationSeconds) || 0);
  const loggedIn = options.loggedIn === true;
  const storage = options.storage;
  const send = options.send;
  const now = typeof options.now === "function" ? options.now : Date.now;
  const throttleMs = Math.min(
    15_000,
    Math.max(10_000, Number(options.throttleMs) || CLASSROOM_PROGRESS_THROTTLE_MS),
  );
  let queued = null;
  let lastSentAt = null;
  let inFlight = null;
  let completed = options.completed === true;

  function snapshot(value) {
    const positionSeconds = position(value, durationSeconds);
    if (!loggedIn)
      completed = completed || classroomCompletion(positionSeconds, durationSeconds).completed;
    return { positionSeconds, completed };
  }

  async function transmit({ flush = false } = {}) {
    if (inFlight) {
      await inFlight;
      return flush && (inFlight || queued) ? transmit({ flush: true }) : null;
    }
    if (!queued) return null;
    if (typeof send !== "function") throw new Error("学习进度同步方法未配置");
    const current = queued;
    queued = null;
    const operation = (async () => {
      try {
        const response = await send(id, current.positionSeconds);
        if (response?.completed === true) completed = true;
        const confirmed = {
          positionSeconds:
            response && Object.prototype.hasOwnProperty.call(response, "positionSeconds")
              ? position(response.positionSeconds, durationSeconds)
              : current.positionSeconds,
          completed,
        };
        lastSentAt = now();
        return confirmed;
      } catch (error) {
        if (!queued) queued = current;
        throw error;
      }
    })();
    let tracked;
    tracked = operation.finally(() => {
      if (inFlight === tracked) inFlight = null;
    });
    inFlight = tracked;
    const result = await tracked;
    if (flush && (inFlight || queued)) return transmit({ flush: true });
    return result;
  }

  return {
    async record(value, { force = false } = {}) {
      const current = snapshot(value);
      if (!loggedIn) {
        const local = { ...current, updatedAt: now() };
        try {
          storage?.setItem(localKey(id), JSON.stringify(local));
        } catch {
          /* 本地存储异常不影响播放 */
        }
        return { ...current, local: true };
      }
      queued = current;
      let confirmed = null;
      if (force) confirmed = await transmit({ flush: true });
      else if (!inFlight && (lastSentAt === null || now() - lastSentAt >= throttleMs))
        confirmed = await transmit();
      return confirmed || current;
    },
    flush() {
      return transmit({ flush: true });
    },
    retry() {
      return transmit({ flush: true });
    },
    pending() {
      return queued ? { ...queued } : null;
    },
  };
}

function paymentCancelled(error) {
  const message = String(error?.errMsg || error?.message || "").toLowerCase();
  return message.includes("cancel") || message.includes("取消");
}

function orderPaid(status) {
  return status?.owned === true || status?.status === "paid";
}

function orderTerminal(status) {
  return ["closed", "cancelled", "canceled", "failed", "refunded"].includes(
    String(status?.status || "").toLowerCase(),
  );
}

export function createClassroomPurchaseController(options = {}) {
  const maxAttempts = Math.max(
    1,
    Math.min(
      CLASSROOM_ORDER_POLL_MAX_ATTEMPTS,
      Math.floor(Number(options.maxAttempts) || CLASSROOM_ORDER_POLL_MAX_ATTEMPTS),
    ),
  );
  const wait =
    typeof options.wait === "function"
      ? options.wait
      : (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const intervalMs = Math.max(0, Number(options.intervalMs) || 1_200);
  let generation = 0;
  let currentOperation = null;
  let current = { state: "idle", message: "", order: null, status: null };

  function publish(state, message = "", extra = {}) {
    current = { ...current, ...extra, state, message };
    options.onChange?.({ ...current });
  }

  function active(run) {
    return generation === run;
  }

  function purchase() {
    if (currentOperation) return currentOperation;
    const run = ++generation;
    const operation = (async () => {
      try {
        publish("creating", "正在创建安全订单…");
        const order = await options.create();
        if (!active(run)) return current;
        publish("pending", "订单已创建，请完成微信支付", { order });
        await options.pay(order);
        if (!active(run)) return current;
        publish("pending", "支付已提交，正在确认结果…", { order });
        for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
          if (attempt > 0) await wait(intervalMs);
          if (!active(run)) return current;
          const status = await options.status(order);
          if (!active(run)) return current;
          if (orderPaid(status)) {
            publish("success", "购买成功，正在刷新课件权限", { status });
            await options.onSuccess?.(status);
            return current;
          }
          if (orderTerminal(status)) {
            publish("failure", "订单未完成，可重新发起支付", { status });
            return current;
          }
        }
        if (active(run)) publish("failure", "支付结果确认超时，请重试查询或重新支付");
      } catch (error) {
        if (!active(run)) return current;
        publish(
          paymentCancelled(error) ? "cancelled" : "failure",
          paymentCancelled(error)
            ? "已取消支付，可稍后继续"
            : String(error?.message || "支付失败，请重试"),
        );
      }
      return current;
    })();
    let tracked;
    tracked = operation.finally(() => {
      if (currentOperation === tracked) currentOperation = null;
    });
    currentOperation = tracked;
    return tracked;
  }

  return {
    purchase,
    retry: purchase,
    stop() {
      generation += 1;
      currentOperation = null;
    },
    reset() {
      generation += 1;
      currentOperation = null;
      publish("idle");
    },
    snapshot() {
      return { ...current };
    },
  };
}
