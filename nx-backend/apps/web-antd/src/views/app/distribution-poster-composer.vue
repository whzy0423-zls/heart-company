<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useAccessStore } from '@vben/stores';
import { Alert, Button, Card, Form, Input, InputNumber, Slider, Space, Typography, Upload, message } from 'ant-design-vue';
import QRCode from 'qrcode';
import { getPosterConfigApi, savePosterConfigApi } from '#/api/core/distribution-poster';
import { uploadFileApi } from '#/api/core/upload';
import { createUploadAssetObjectURL } from '#/utils/upload-asset-preview';

import { clampPosterPosition, dragPosterPosition } from './distribution-poster-layout';

const props = defineProps<{ agentCode?: string; editable?: boolean }>();
const access = useAccessStore();
const canEdit = computed(() => !!props.editable && access.accessCodes.includes('Customer:App:Write'));
const canvasRef = ref<HTMLCanvasElement>();
const inviteCode = ref(props.agentCode || '');
const landingUrl = ref('');
const templateUrl = ref('');
const qrImageUrl = ref('');
const qrSize = ref(176);
const qrX = ref(62);
const qrY = ref(1010);
const inviteX = ref(286);
const inviteY = ref(1100);
const inviteWidth = ref(350);
const inviteFontSize = ref(26);
const previewRef = ref<HTMLDivElement>();
let drag: { kind: 'qr' | 'invite'; pointer: number; x: number; y: number; clientX: number; clientY: number; width: number; height: number } | undefined;
function boxStyle(kind: 'qr' | 'invite') {
  const qr = kind === 'qr';
  return {
    left: ((qr ? qrX.value : inviteX.value) / 720 * 100) + '%',
    top: ((qr ? qrY.value : inviteY.value) / 1280 * 100) + '%',
    width: ((qr ? qrSize.value : inviteWidth.value) / 720 * 100) + '%',
    height: ((qr ? qrSize.value : inviteFontSize.value + 12) / 1280 * 100) + '%',
  };
}
function moveElement(kind: 'qr' | 'invite', x: number, y: number) {
  const qr = kind === 'qr';
  const pos = clampPosterPosition(x, y, qr ? qrSize.value : inviteWidth.value, qr ? qrSize.value : inviteFontSize.value + 12);
  if (qr) { qrX.value = pos.x; qrY.value = pos.y; }
  else { inviteX.value = pos.x; inviteY.value = pos.y; }
}
function startDrag(event: PointerEvent, kind: 'qr' | 'invite') {
  if (!canEdit.value || !templateUrl.value || event.button !== 0) return;
  const rect = previewRef.value?.getBoundingClientRect();
  if (!rect) return;
  event.preventDefault();
  drag = { kind, pointer: event.pointerId, x: kind === 'qr' ? qrX.value : inviteX.value, y: kind === 'qr' ? qrY.value : inviteY.value,
    clientX: event.clientX, clientY: event.clientY, width: rect.width, height: rect.height };
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
}
function moveDrag(event: PointerEvent) {
  if (!drag || drag.pointer !== event.pointerId || !canEdit.value) return;
  const qr = drag.kind === 'qr';
  const pos = dragPosterPosition(drag.x, drag.y, event.clientX - drag.clientX, event.clientY - drag.clientY,
    drag.width, drag.height, qr ? qrSize.value : inviteWidth.value, qr ? qrSize.value : inviteFontSize.value + 12);
  moveElement(drag.kind, pos.x, pos.y);
}
function stopDrag() { drag = undefined; }
function nudge(event: KeyboardEvent, kind: 'qr' | 'invite') {
  if (!canEdit.value) return;
  const delta = ({ ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] } as Record<string, number[]>)[event.key];
  if (!delta) return;
  event.preventDefault();
  const step = event.shiftKey ? 10 : 1;
  moveElement(kind, (kind === 'qr' ? qrX.value : inviteX.value) + delta[0]! * step, (kind === 'qr' ? qrY.value : inviteY.value) + delta[1]! * step);
}
const loading = ref(true);
const saving = ref(false);
const uploading = ref(false);
const rendering = ref(false);
const ready = ref(false);
const loadError = ref(false);
const renderError = ref('');
let renderVersion = 0;
let disposed = false;
const imageCache = new Map<string, Promise<HTMLImageElement>>();
const objectURLs = new Set<string>();

async function loadImage(source: string) {
  if (!imageCache.has(source)) {
    const pending = (async () => {
      const url = await createUploadAssetObjectURL(source, access.accessToken);
      if (!url) throw new Error('图片加载失败');
      if (url.startsWith('blob:')) objectURLs.add(url);
      return new Promise<HTMLImageElement>((resolve, reject) => {
        const image = new Image();
        image.onload = () => resolve(image);
        image.onerror = () => reject(new Error('图片加载失败'));
        image.src = url;
      });
    })();
    imageCache.set(source, pending);
    pending.catch(() => imageCache.delete(source));
  }
  return imageCache.get(source)!;
}

async function upload(file: File, kind: 'template' | 'qr') {
  if (!canEdit.value) return false;
  if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type) || file.size > 12 * 1024 * 1024) {
    message.error('请选择不超过 12MB 的 PNG、JPG 或 WebP 图片');
    return false;
  }
  uploading.value = true;
  try {
    const result = await uploadFileApi(file, 'poster');
    await loadImage(result.url);
    if (kind === 'template') templateUrl.value = result.url;
    else qrImageUrl.value = result.url;
    message.success('图片已上传，请保存配置发布给代理');
  } catch { message.error('图片上传失败，请重试'); }
  finally { uploading.value = false; }
  return false;
}

async function loadConfig() {
  loading.value = true;
  loadError.value = false;
  try {
    const cfg = await getPosterConfigApi();
    templateUrl.value = cfg.templateUrl || '';
    landingUrl.value = cfg.landingUrl || '';
    qrImageUrl.value = cfg.qrImageUrl || '';
    qrSize.value = cfg.qrSize || 176;
    qrX.value = cfg.qrX ?? 62; qrY.value = cfg.qrY ?? 1010;
    inviteX.value = cfg.inviteX ?? 286; inviteY.value = cfg.inviteY ?? 1100;
    inviteWidth.value = cfg.inviteWidth ?? 350; inviteFontSize.value = cfg.inviteFontSize ?? 26;
  } catch { loadError.value = true; }
  finally { loading.value = false; }
  await nextTick();
  await render();
}

async function saveConfig() {
  if (!canEdit.value || !ready.value || uploading.value) return;
  saving.value = true;
  try {
    await savePosterConfigApi({
      qrX: qrX.value, qrY: qrY.value, inviteX: inviteX.value, inviteY: inviteY.value, inviteWidth: inviteWidth.value, inviteFontSize: inviteFontSize.value,
      templateUrl: templateUrl.value, landingUrl: landingUrl.value.trim(), qrImageUrl: qrImageUrl.value, qrSize: qrSize.value,
    });
    message.success('海报配置已发布，代理重新打开页面即可使用');
  } catch { message.error('保存失败，请检查模板和二维码设置'); }
  finally { saving.value = false; }
}

function text(ctx: CanvasRenderingContext2D, value: string, x: number, y: number, maxWidth: number) {
  ctx.fillText(value, x, y, maxWidth);
}

async function render() {
  const version = ++renderVersion;
  ready.value = false;
  renderError.value = '';
  if (loading.value || loadError.value || !canvasRef.value) return;
  const visible = canvasRef.value;
  if (!templateUrl.value) {
    visible.getContext('2d')?.clearRect(0, 0, visible.width, visible.height);
    rendering.value = false;
    return;
  }
  rendering.value = true;
  try {
    const background = await loadImage(templateUrl.value);
    let qrSource = qrImageUrl.value;
    if (!qrSource && landingUrl.value.trim()) {
      const url = new URL(landingUrl.value.trim());
      if (!['http:', 'https:'].includes(url.protocol)) throw new Error('请输入有效的二维码链接');
      // The administrator owns the QR destination; invitation text never changes it.
      qrSource = await QRCode.toDataURL(landingUrl.value.trim(), { margin: 4, width: 660, errorCorrectionLevel: 'M' });
    }
    const qr = qrSource && !qrImageUrl.value ? await new Promise<HTMLImageElement>((resolve, reject) => {
      const image = new Image();
      image.onload = () => resolve(image);
      image.onerror = () => reject(new Error('二维码生成失败'));
      image.src = qrSource;
    }) : qrImageUrl.value ? await loadImage(qrSource) : undefined;
    const canvas = document.createElement('canvas');
    canvas.width = 720; canvas.height = 1280;
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new Error('浏览器不支持海报绘制');
    const scale = Math.max(720 / background.naturalWidth, 1280 / background.naturalHeight);
    const w = background.naturalWidth * scale, h = background.naturalHeight * scale;
    ctx.drawImage(background, (720-w)/2, (1280-h)/2, w, h);
    const size = qrSize.value;
    ctx.fillStyle = '#fff'; ctx.fillRect(qrX.value, qrY.value, size, size);
    if (qr) {
      const qrScale = Math.min(size / qr.naturalWidth, size / qr.naturalHeight);
      const qw = qr.naturalWidth * qrScale, qh = qr.naturalHeight * qrScale;
      ctx.drawImage(qr, qrX.value+(size-qw)/2, qrY.value+(size-qh)/2, qw, qh);
    } else {
      ctx.strokeStyle = '#2563eb'; ctx.setLineDash([8, 6]); ctx.strokeRect(qrX.value, qrY.value, size, size); ctx.setLineDash([]);
      ctx.fillStyle = '#2563eb'; ctx.font = '600 16px sans-serif'; ctx.textAlign = 'center';
      ctx.fillText('二维码待配置', qrX.value + size / 2, qrY.value + size / 2);
      ctx.textAlign = 'start';
    }
    ctx.fillStyle = '#111827';
    ctx.textBaseline = 'top';
    ctx.font = '700 ' + inviteFontSize.value + 'px sans-serif';
    text(ctx, inviteCode.value.trim() || '邀请码', inviteX.value, inviteY.value + 6, inviteWidth.value);
    if (disposed || version !== renderVersion) return;
    visible.width = 720; visible.height = 1280;
    visible.getContext('2d')?.drawImage(canvas, 0, 0);
    ready.value = true;
  } catch (error) {
    if (version === renderVersion) renderError.value = error instanceof Error ? error.message : '海报预览生成失败';
  } finally { if (version === renderVersion) rendering.value = false; }
}

function downloadPoster() {
  if (!ready.value || rendering.value || !inviteCode.value.trim() || !canvasRef.value) return;
  try {
    const link = document.createElement('a');
    link.download = '芯之力-代理海报.png';
    link.href = canvasRef.value.toDataURL('image/png');
    link.click();
  } catch { message.error('海报导出失败，请重新加载图片'); }
}

watch([inviteCode, landingUrl, qrImageUrl, templateUrl, qrSize, qrX, qrY, inviteX, inviteY, inviteWidth, inviteFontSize], () => void render());
watch([qrSize, inviteWidth, inviteFontSize], () => {
  moveElement('qr', qrX.value, qrY.value);
  moveElement('invite', inviteX.value, inviteY.value);
});
watch(() => props.agentCode, code => { if (code) inviteCode.value = code; });
onMounted(async () => { await nextTick(); await loadConfig(); });
onBeforeUnmount(() => {
  disposed = true; renderVersion++;
  objectURLs.forEach(url => URL.revokeObjectURL(url));
});
</script>

<template>
  <Card :bordered="false" class="poster-composer-card" :loading="loading">
    <template #title>{{ canEdit ? '海报模板与二维码设置' : '生成我的分享海报' }}</template>
    <template #extra>
      <Space wrap>
        <Button @click="loadConfig">重新加载</Button>
        <Button v-if="canEdit" type="primary" :loading="saving" :disabled="!ready || rendering || uploading || (!landingUrl.trim() && !qrImageUrl)" @click="saveConfig">保存并发布</Button>
        <Button :disabled="!ready || rendering || !inviteCode.trim()" @click="downloadPoster">生成并下载 PNG</Button>
      </Space>
    </template>
    <Alert v-if="loadError" type="error" show-icon message="海报配置加载失败，请重新加载" />
    <Alert v-else-if="!templateUrl || (!landingUrl && !qrImageUrl)" type="info" show-icon :message="canEdit ? '请上传模板并设置固定二维码，保存后代理即可使用' : '管理员尚未发布海报，请稍后重试'" />
    <Alert v-if="renderError" type="error" show-icon :message="renderError" />
    <div class="poster-composer-layout">
      <div class="poster-preview-shell">
        <div ref="previewRef" class="poster-stage">
          <canvas ref="canvasRef" class="poster-canvas" width="720" height="1280"></canvas>
          <template v-if="canEdit && templateUrl">
            <button v-for="kind in (['qr', 'invite'] as const)" :key="kind" type="button" class="poster-drag-target" :data-element="kind" :style="boxStyle(kind)"
              :aria-label="kind === 'qr' ? '拖动二维码，方向键微调' : '拖动邀请码，方向键微调'"
              @pointerdown="startDrag($event, kind)" @pointermove="moveDrag" @pointerup="stopDrag" @pointercancel="stopDrag" @lostpointercapture="stopDrag" @keydown="nudge($event, kind)">
              <span>{{ kind === 'qr' ? '二维码 · 拖动' : '邀请码 · 拖动' }}</span>
            </button>
          </template>
        </div>
        <Typography.Text v-if="canEdit" type="secondary">直接拖动蓝色框调整位置；方向键微调，Shift + 方向键移动 10px。蓝色框不会出现在下载的海报中。</Typography.Text>
        <Typography.Text type="secondary">二维码由管理端统一设置，填写邀请码不会改变二维码</Typography.Text>
      </div>
      <Form layout="vertical" class="poster-form">
        <template v-if="canEdit">
          <Form.Item label="海报模板（上传后立即预览）">
            <Upload accept="image/png,image/jpeg,image/webp" :show-upload-list="false" :disabled="uploading || saving" :before-upload="file => upload(file, 'template')"><Button :loading="uploading">上传海报模板</Button></Upload>
          </Form.Item>
          <Form.Item label="二维码（上传图片或填写链接二选一）">
            <Space>
              <Upload accept="image/png,image/jpeg,image/webp" :show-upload-list="false" :disabled="uploading || saving" :before-upload="file => upload(file, 'qr')"><Button>上传二维码</Button></Upload>
              <Button v-if="qrImageUrl" @click="qrImageUrl = ''">移除二维码图片</Button>
            </Space>
          </Form.Item>
          <Form.Item label="固定二维码链接（未上传二维码图片时使用）"><Input v-model:value="landingUrl" placeholder="https://..." :maxlength="2048" /></Form.Item>
          <Form.Item label="二维码位置（X / Y）"><Space><InputNumber v-model:value="qrX" :min="0" :max="720 - qrSize" :precision="0" /><InputNumber v-model:value="qrY" :min="0" :max="1280 - qrSize" :precision="0" /></Space></Form.Item>
          <Form.Item label="邀请码位置（X / Y）"><Space><InputNumber v-model:value="inviteX" :min="0" :max="720 - inviteWidth" :precision="0" /><InputNumber v-model:value="inviteY" :min="0" :max="1268 - inviteFontSize" :precision="0" /></Space></Form.Item>
          <Form.Item label="邀请码字号"><Slider v-model:value="inviteFontSize" :min="16" :max="64" /></Form.Item>
          <Form.Item label="邀请码区域宽度"><Slider v-model:value="inviteWidth" :min="120" :max="600" /></Form.Item>
          <Form.Item label="二维码尺寸"><Slider v-model:value="qrSize" :min="132" :max="220" :step="4" /></Form.Item>
        </template>
        <Form.Item :label="canEdit ? '预览邀请码（不保存）' : '邀请码'"><Input v-model:value="inviteCode" :maxlength="32" placeholder="请输入邀请码" /></Form.Item>
        <Typography.Paragraph type="secondary">{{ canEdit ? '保存并发布后，模板、文字和二维码统一供代理使用。' : '只需填写邀请码，再下载分享海报。' }}</Typography.Paragraph>
      </Form>
    </div>
  </Card>
</template>

<style scoped>
.poster-composer-card {
  overflow: hidden;
  border: 1px solid hsl(var(--border));
  background: linear-gradient(135deg, rgb(245 158 11 / 7%), rgb(37 99 235 / 5%)), hsl(var(--card));
}
.poster-composer-layout {
  display: grid;
  grid-template-columns: minmax(280px, 420px) minmax(260px, 1fr);
  gap: 28px;
  align-items: start;
}
.poster-preview-shell {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: center;
}
.poster-stage { position: relative; width: min(100%, 390px); }
.poster-drag-target { position: absolute; z-index: 1; border: 2px dashed #2563eb; background: transparent; cursor: grab; touch-action: none; padding: 0; }
.poster-drag-target:active { cursor: grabbing; }
.poster-drag-target:focus-visible { outline: 3px solid #f59e0b; }
.poster-drag-target span { position: absolute; left: 0; top: 0; color: white; background: #2563eb; font-size: 11px; white-space: nowrap; pointer-events: none; }
.poster-canvas {
  display: block;
  width: min(100%, 390px);
  height: auto;
  outline: 1px solid hsl(var(--border));
  box-shadow: 0 16px 36px rgb(15 23 42 / 15%);
}
.poster-form {
  max-width: 560px;
}
.poster-hint {
  margin-top: 12px;
}
@media (max-width: 800px) {
  .poster-composer-layout {
    grid-template-columns: 1fr;
  }
  .poster-form {
    max-width: none;
  }
}
</style>
