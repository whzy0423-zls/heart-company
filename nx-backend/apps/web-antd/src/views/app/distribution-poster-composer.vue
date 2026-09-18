<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useAccessStore } from '@vben/stores';
import { Alert, Button, Card, Form, Input, Slider, Space, Typography, Upload, message } from 'ant-design-vue';
import QRCode from 'qrcode';
import { getPosterConfigApi, savePosterConfigApi } from '#/api/core/distribution-poster';
import { uploadFileApi } from '#/api/core/upload';
import { createUploadAssetObjectURL } from '#/utils/upload-asset-preview';

const props = defineProps<{ agentCode?: string; editable?: boolean }>();
const access = useAccessStore();
const canEdit = computed(() => !!props.editable && access.accessCodes.includes('Customer:App:Write'));
const canvasRef = ref<HTMLCanvasElement>();
const headline = ref('');
const subtitle = ref('');
const cta = ref('');
const inviteCode = ref(props.agentCode || '');
const landingUrl = ref('');
const templateUrl = ref('');
const qrImageUrl = ref('');
const qrSize = ref(176);
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
    headline.value = cfg.headline || '';
    subtitle.value = cfg.subtitle || '';
    cta.value = cfg.cta || '';
    landingUrl.value = cfg.landingUrl || '';
    qrImageUrl.value = cfg.qrImageUrl || '';
    qrSize.value = cfg.qrSize || 176;
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
      templateUrl: templateUrl.value, headline: headline.value, subtitle: subtitle.value,
      cta: cta.value, landingUrl: landingUrl.value.trim(), qrImageUrl: qrImageUrl.value, qrSize: qrSize.value,
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
  if (!templateUrl.value || (!qrImageUrl.value && !landingUrl.value.trim())) {
    visible.getContext('2d')?.clearRect(0, 0, visible.width, visible.height);
    rendering.value = false;
    return;
  }
  rendering.value = true;
  try {
    const background = await loadImage(templateUrl.value);
    let qrSource = qrImageUrl.value;
    if (!qrSource) {
      const url = new URL(landingUrl.value.trim());
      if (!['http:', 'https:'].includes(url.protocol)) throw new Error('请输入有效的二维码链接');
      // The administrator owns the QR destination; invitation text never changes it.
      qrSource = await QRCode.toDataURL(landingUrl.value.trim(), { margin: 4, width: 660, errorCorrectionLevel: 'M' });
    }
    const qr = qrImageUrl.value ? await loadImage(qrSource) : await new Promise<HTMLImageElement>((resolve, reject) => {
      const image = new Image();
      image.onload = () => resolve(image);
      image.onerror = () => reject(new Error('二维码生成失败'));
      image.src = qrSource;
    });
    const canvas = document.createElement('canvas');
    canvas.width = 720; canvas.height = 1280;
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new Error('浏览器不支持海报绘制');
    const scale = Math.max(720 / background.naturalWidth, 1280 / background.naturalHeight);
    const w = background.naturalWidth * scale, h = background.naturalHeight * scale;
    ctx.drawImage(background, (720-w)/2, (1280-h)/2, w, h);
    ctx.fillStyle = 'rgba(255,255,255,0.94)';
    ctx.beginPath(); ctx.roundRect(34, 916, 652, 330, 28); ctx.fill();
    ctx.fillStyle = '#1f2937'; ctx.font = '700 32px sans-serif';
    text(ctx, headline.value, 62, 963, 596);
    ctx.fillStyle = '#667085'; ctx.font = '18px sans-serif';
    text(ctx, subtitle.value, 62, 998, 596);
    const size = qrSize.value;
    ctx.fillStyle = '#fff'; ctx.fillRect(62, 1010, size, size);
    const qrScale = Math.min(size / qr.naturalWidth, size / qr.naturalHeight);
    const qw = qr.naturalWidth * qrScale, qh = qr.naturalHeight * qrScale;
    ctx.drawImage(qr, 62+(size-qw)/2, 1010+(size-qh)/2, qw, qh);
    const x = 62 + size + 24, available = 658-x;
    ctx.fillStyle = '#111827'; ctx.font = '600 21px sans-serif';
    text(ctx, '新用户专属邀请码', x, 1070, available);
    ctx.font = '700 26px sans-serif';
    text(ctx, inviteCode.value.trim() || '邀请码', x, 1120, available);
    ctx.font = '600 20px sans-serif';
    text(ctx, cta.value, x, 1180, available);
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

watch([headline, subtitle, cta, inviteCode, landingUrl, qrImageUrl, templateUrl, qrSize], () => void render());
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
        <Button v-if="canEdit" type="primary" :loading="saving" :disabled="!ready || rendering || uploading" @click="saveConfig">保存并发布</Button>
        <Button :disabled="!ready || rendering || !inviteCode.trim()" @click="downloadPoster">生成并下载 PNG</Button>
      </Space>
    </template>
    <Alert v-if="loadError" type="error" show-icon message="海报配置加载失败，请重新加载" />
    <Alert v-else-if="!templateUrl || (!landingUrl && !qrImageUrl)" type="info" show-icon :message="canEdit ? '请上传模板并设置固定二维码，保存后代理即可使用' : '管理员尚未发布海报，请稍后重试'" />
    <Alert v-if="renderError" type="error" show-icon :message="renderError" />
    <div class="poster-composer-layout">
      <div class="poster-preview-shell">
        <canvas ref="canvasRef" class="poster-canvas" width="720" height="1280"></canvas>
        <Typography.Text type="secondary">二维码由管理端统一设置，填写邀请码不会改变二维码</Typography.Text>
      </div>
      <Form layout="vertical" class="poster-form">
        <template v-if="canEdit">
          <Form.Item label="海报模板">
            <Upload accept="image/png,image/jpeg,image/webp" :show-upload-list="false" :disabled="uploading || saving" :before-upload="file => upload(file, 'template')"><Button :loading="uploading">上传海报模板</Button></Upload>
          </Form.Item>
          <Form.Item label="主标题"><Input v-model:value="headline" :maxlength="32" /></Form.Item>
          <Form.Item label="副文案"><Input.TextArea v-model:value="subtitle" :maxlength="80" :rows="2" /></Form.Item>
          <Form.Item label="行动文案"><Input v-model:value="cta" :maxlength="24" /></Form.Item>
          <Form.Item label="固定二维码图片（优先使用）">
            <Space>
              <Upload accept="image/png,image/jpeg,image/webp" :show-upload-list="false" :disabled="uploading || saving" :before-upload="file => upload(file, 'qr')"><Button>上传二维码</Button></Upload>
              <Button v-if="qrImageUrl" @click="qrImageUrl = ''">移除二维码图片</Button>
            </Space>
          </Form.Item>
          <Form.Item label="固定二维码链接（未上传二维码图片时使用）"><Input v-model:value="landingUrl" placeholder="https://..." :maxlength="2048" /></Form.Item>
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
.poster-canvas {
  display: block;
  width: min(100%, 390px);
  height: auto;
  border: 1px solid hsl(var(--border));
  border-radius: 12px;
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
