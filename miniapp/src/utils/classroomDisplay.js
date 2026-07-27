const ACCESS_LEVELS = new Set(["public", "login", "member", "paid"]);
const CONTENT_ACCESS_LEVELS = new Set(["inherit", "public", "login", "member", "paid"]);
const PURCHASE_STATES = new Set(["available", "owned", "purchase_required"]);

function text(value) {
  return typeof value === "string" ? value.trim() : "";
}

function id(value) {
  const normalized = String(value ?? "").trim();
  return /^\d+$/.test(normalized) && normalized !== "0" ? normalized : "";
}

function nonNegativeInteger(value) {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? Math.floor(number) : 0;
}

function access(value) {
  return ACCESS_LEVELS.has(value) ? value : "";
}

function purchaseState(value, effectiveAccess, canPlay) {
  if (PURCHASE_STATES.has(value)) return value;
  if (effectiveAccess !== "paid") return "available";
  return canPlay ? "owned" : "purchase_required";
}

export function normalizeClassroomSeries(source = {}) {
  const effectiveAccess = access(source.effectiveAccess);
  const canPlay = source.canPlay === true;
  return {
    id: id(source.id),
    title: text(source.title),
    summary: text(source.summary),
    coverUrl: text(source.coverUrl),
    teacherName: text(source.teacherName),
    effectiveAccess,
    priceCents: nonNegativeInteger(source.priceCents),
    canPlay,
    purchaseState: purchaseState(source.purchaseState, effectiveAccess, canPlay),
    playbackBlocked: source.playbackBlocked === true,
  };
}

export function normalizeClassroomContent(source = {}) {
  const effectiveAccess = access(source.effectiveAccess);
  const canPlay = source.canPlay === true;
  return {
    id: id(source.id),
    seriesId: id(source.seriesId),
    title: text(source.title),
    description: text(source.description),
    teacherName: text(source.teacherName),
    coverUrl: text(source.coverUrl),
    contentType: source.contentType === "audio" ? "audio" : "video",
    durationSeconds: nonNegativeInteger(source.durationSeconds),
    accessLevel: CONTENT_ACCESS_LEVELS.has(source.accessLevel) ? source.accessLevel : "",
    effectiveAccess,
    priceCents: nonNegativeInteger(source.priceCents),
    canPlay,
    purchaseState: purchaseState(source.purchaseState, effectiveAccess, canPlay),
    playbackBlocked: source.playbackBlocked === true,
  };
}

export function classroomAccessLabel(value) {
  return (
    { public: "免费", login: "登录可学", member: "会员专享", paid: "付费课件" }[value] ||
    "权限待确认"
  );
}

export function classroomPurchaseAction(item = {}) {
  if (item.playbackBlocked) return { type: "blocked", label: "暂不可播放" };
  if (item.canPlay)
    return { type: "play", label: item.purchaseState === "owned" ? "继续学习" : "立即学习" };
  if (item.purchaseState === "purchase_required" || item.effectiveAccess === "paid") {
    const price = nonNegativeInteger(item.priceCents);
    return { type: "purchase", label: price ? `¥${(price / 100).toFixed(2)} 购买` : "立即购买" };
  }
  if (item.effectiveAccess === "login") return { type: "login", label: "登录后学习" };
  if (item.effectiveAccess === "member") return { type: "member", label: "开通会员" };
  return { type: "unavailable", label: "暂不可学习" };
}

export function classroomContentRoute(item = {}) {
  const contentId = id(item.id);
  if (!contentId) return "";
  const type = item.contentType === "audio" ? "audio" : "video";
  return `/pages/classroom-detail/classroom-detail?id=${contentId}&type=${type}`;
}
