const POSTER_WIDTH = 320;
const POSTER_HEIGHT = 460;

export function wrapCanvasText(ctx, text, x, y, maxWidth, lineHeight) {
  const chars = String(text || "").split("");
  let line = "";
  let lineY = y;
  for (const character of chars) {
    const nextLine = line + character;
    if (line && ctx.measureText(nextLine).width > maxWidth) {
      ctx.fillText(line, x, lineY);
      line = character;
      lineY += lineHeight;
    } else {
      line = nextLine;
    }
  }
  if (line) ctx.fillText(line, x, lineY);
}

function findPosterCanvas(runtime, instance) {
  return new Promise((resolve, reject) => {
    let query = runtime.createSelectorQuery();
    if (typeof query.in === "function") query = query.in(instance?.proxy || instance);
    query
      .select("#poster-canvas")
      .fields({ node: true, size: true })
      .exec((result) => {
        const canvas = result?.[0]?.node;
        if (!canvas) {
          reject(new Error("海报画布未找到"));
          return;
        }
        resolve(canvas);
      });
  });
}

function loadCanvasImage(canvas, src) {
  return new Promise((resolve, reject) => {
    const image = canvas.createImage();
    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error("海报头像加载失败"));
    image.src = src;
  });
}

function exportCanvas(runtime, canvas, instance) {
  return new Promise((resolve, reject) => {
    runtime.canvasToTempFilePath(
      {
        canvas,
        success: (result) => {
          if (typeof result?.tempFilePath !== "string" || !result.tempFilePath) {
            reject(new Error("海报导出路径缺失"));
            return;
          }
          resolve(result.tempFilePath);
        },
        fail: reject,
      },
      instance?.proxy || instance,
    );
  });
}

function resolvePosterRuntime(runtime) {
  const resolvedRuntime = runtime || globalThis.uni;
  if (
    !resolvedRuntime ||
    typeof resolvedRuntime.createSelectorQuery !== "function" ||
    typeof resolvedRuntime.canvasToTempFilePath !== "function"
  ) {
    throw new Error("海报运行环境不可用");
  }
  return resolvedRuntime;
}

export async function createResultPoster({
  instance,
  result,
  info,
  summary,
  title,
  runtime,
}) {
  const activeRuntime = resolvePosterRuntime(runtime);
  const canvas = await findPosterCanvas(activeRuntime, instance);
  const ctx = canvas.getContext("2d");
  const dpr = activeRuntime.getSystemInfoSync?.().pixelRatio || 2;
  canvas.width = POSTER_WIDTH * dpr;
  canvas.height = POSTER_HEIGHT * dpr;
  ctx.scale(dpr, dpr);

  const type = result.type;
  const accent = { green: "#38a83a", blue: "#1f73c4", red: "#e23a2f" }[info.color] || "#1f73c4";
  ctx.fillStyle = "#ffffff";
  ctx.fillRect(0, 0, POSTER_WIDTH, POSTER_HEIGHT);
  ctx.fillStyle = accent;
  ctx.fillRect(0, 0, POSTER_WIDTH, 6);

  ctx.textAlign = "center";
  ctx.fillStyle = "#9aa7b5";
  ctx.font = "12px sans-serif";
  ctx.fillText("九型芯之力 · 性格芯片测试", POSTER_WIDTH / 2, 34);

  const avatar = await loadCanvasImage(canvas, `/static/avatars/${type}.png`);
  const centerX = POSTER_WIDTH / 2;
  const centerY = 110;
  const radius = 52;
  ctx.save();
  ctx.beginPath();
  ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);
  ctx.clip();
  ctx.drawImage(avatar, centerX - radius, centerY - radius, radius * 2, radius * 2);
  ctx.restore();
  ctx.beginPath();
  ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);
  ctx.lineWidth = 3;
  ctx.strokeStyle = accent;
  ctx.stroke();

  ctx.fillStyle = "#1a2430";
  ctx.font = "bold 24px sans-serif";
  ctx.fillText(`${type}号 · ${title}`, POSTER_WIDTH / 2, 195);
  ctx.fillStyle = accent;
  ctx.font = "bold 13px sans-serif";
  ctx.fillText(`${info.en} · ${info.keywords}`, POSTER_WIDTH / 2, 220);

  ctx.fillStyle = "#42505e";
  ctx.font = "14px sans-serif";
  wrapCanvasText(ctx, summary, POSTER_WIDTH / 2, 252, POSTER_WIDTH - 56, 22);

  ctx.fillStyle = "#f4f7f9";
  ctx.fillRect(0, POSTER_HEIGHT - 80, POSTER_WIDTH, 80);
  ctx.fillStyle = "#1a2430";
  ctx.font = "bold 14px sans-serif";
  ctx.fillText("长按识别 · 测测你是哪一块性格芯片", POSTER_WIDTH / 2, POSTER_HEIGHT - 46);
  ctx.fillStyle = accent;
  ctx.font = "12px sans-serif";
  ctx.fillText("微信搜索「九型芯之力」小程序", POSTER_WIDTH / 2, POSTER_HEIGHT - 24);

  return exportCanvas(activeRuntime, canvas, instance);
}
