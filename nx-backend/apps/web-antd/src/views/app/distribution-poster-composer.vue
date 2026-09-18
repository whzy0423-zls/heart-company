<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { Button, Card, Form, Input, Slider, Space, Typography, Upload, message } from 'ant-design-vue';
import type { UploadProps } from 'ant-design-vue';
import QRCode from 'qrcode';

const props = defineProps<{ agentCode?: string }>();

const canvasRef = ref<HTMLCanvasElement>();
const fileInputRef = ref<HTMLInputElement>();
const image = ref<HTMLImageElement>();
const imageUrl = ref('');
const headline = ref('看见自己，也读懂关系');
const subtitle = ref('从人格画像到日常陪伴，让每一次觉察都成为成长的开始。');
const cta = ref('立即开启自我探索');
const inviteCode = ref(props.agentCode || 'INVITE_CODE');
const landingUrl = ref('');
const qrSize = ref(176);
const rendering = ref(false);

const resolvedUrl = computed(() =>
  landingUrl.value.trim() || `https://xinzhili.app/register?agentCode=${encodeURIComponent(inviteCode.value.trim())}`,
);

function resetInviteCode() {
  if (props.agentCode) inviteCode.value = props.agentCode;
}

function openFilePicker() {
  fileInputRef.value?.click();
}

function handleFile(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0];
  if (!file) return;
  if (!file.type.startsWith('image/')) {
    message.error('请选择 PNG、JPG 或 WebP 图片');
    return;
  }
  if (file.size > 12 * 1024 * 1024) {
    message.error('图片大小不能超过 12MB');
    return;
  }
  const nextUrl = URL.createObjectURL(file);
  if (imageUrl.value) URL.revokeObjectURL(imageUrl.value);
  imageUrl.value = nextUrl;
  const nextImage = new Image();
  nextImage.onload = () => {
    image.value = nextImage;
    void render();
  };
  nextImage.src = nextUrl;
}

const uploadProps: UploadProps = {
  accept: 'image/png,image/jpeg,image/webp',
  showUploadList: false,
  beforeUpload: (file) => {
    if (file instanceof File) {
      const dataTransfer = new DataTransfer();
      dataTransfer.items.add(file);
      if (fileInputRef.value) fileInputRef.value.files = dataTransfer.files;
      handleFile({ target: { files: dataTransfer.files } } as unknown as Event);
    }
    return false;
  },
};

function drawCover(ctx: CanvasRenderingContext2D, source: CanvasImageSource, width: number, height: number) {
  const sourceWidth = source instanceof HTMLImageElement ? source.naturalWidth : width;
  const sourceHeight = source instanceof HTMLImageElement ? source.naturalHeight : height;
  const scale = Math.max(width / sourceWidth, height / sourceHeight);
  const drawWidth = sourceWidth * scale;
  const drawHeight = sourceHeight * scale;
  ctx.drawImage(source, (width - drawWidth) / 2, (height - drawHeight) / 2, drawWidth, drawHeight);
}

function roundedRect(ctx: CanvasRenderingContext2D, x: number, y: number, width: number, height: number, radius: number) {
  ctx.beginPath();
  ctx.roundRect(x, y, width, height, radius);
  ctx.closePath();
}

async function render() {
  const canvas = canvasRef.value;
  if (!canvas) return;
  rendering.value = true;
  try {
    const width = 720;
    const height = 1280;
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    ctx.fillStyle = '#f8f4ee';
    ctx.fillRect(0, 0, width, height);
    if (image.value) drawCover(ctx, image.value, width, height);
    else {
      const gradient = ctx.createLinearGradient(0, 0, width, height);
      gradient.addColorStop(0, '#f8f1e7');
      gradient.addColorStop(1, '#e8edf5');
      ctx.fillStyle = gradient;
      ctx.fillRect(0, 0, width, height);
    }
    ctx.fillStyle = 'rgba(255, 255, 255, 0.88)';
    roundedRect(ctx, 34, height - 338, width - 68, 286, 28);
    ctx.fill();
    ctx.fillStyle = '#1f2937';
    ctx.font = '700 32px sans-serif';
    ctx.fillText(headline.value.trim() || '看见自己，也读懂关系', 62, height - 290);
    ctx.fillStyle = '#667085';
    ctx.font = '20px sans-serif';
    const description = subtitle.value.trim() || '从人格画像到日常陪伴，让每一次觉察都成为成长的开始。';
    ctx.fillText(description.slice(0, 30), 62, height - 254);
    ctx.fillStyle = '#111827';
    ctx.font = '600 21px sans-serif';
    ctx.fillText('新用户专属邀请码', 278, height - 200);
    ctx.fillStyle = '#667085';
    ctx.font = '18px sans-serif';
    ctx.fillText(inviteCode.value.trim() || 'INVITE_CODE', 278, height - 165);
    ctx.fillStyle = '#111827';
    ctx.font = '700 21px sans-serif';
    ctx.fillText(cta.value.trim() || '立即开启自我探索', 278, height - 112);
    const qrDataUrl = await QRCode.toDataURL(resolvedUrl.value, { margin: 1, width: qrSize.value, errorCorrectionLevel: 'M' });
    const qrImage = new Image();
    await new Promise<void>((resolve, reject) => {
      qrImage.onload = () => resolve();
      qrImage.onerror = () => reject(new Error('二维码生成失败'));
      qrImage.src = qrDataUrl;
    });
    ctx.fillStyle = '#fff';
    roundedRect(ctx, 62, height - 238, qrSize.value + 18, qrSize.value + 18, 12);
    ctx.fill();
    ctx.drawImage(qrImage, 71, height - 229, qrSize.value, qrSize.value);
  } catch {
    message.error('海报预览生成失败');
  } finally {
    rendering.value = false;
  }
}

function downloadPoster() {
  const canvas = canvasRef.value;
  if (!canvas) return;
  const link = document.createElement('a');
  link.download = `芯之力-代理海报-${inviteCode.value.trim() || 'invite'}.png`;
  link.href = canvas.toDataURL('image/png');
  link.click();
  message.success('海报已生成并下载');
}

watch([headline, subtitle, cta, inviteCode, landingUrl, qrSize], () => void render());
watch(() => props.agentCode, resetInviteCode);

onMounted(async () => {
  await nextTick();
  await render();
});

onBeforeUnmount(() => {
  if (imageUrl.value) URL.revokeObjectURL(imageUrl.value);
});
</script>

<template>
  <Card :bordered="false" class="poster-composer-card">
    <template #title>分享海报生成器</template>
    <template #extra>
      <Space>
        <Upload v-bind="uploadProps">
          <Button>上传海报模板</Button>
        </Upload>
        <Button type="primary" :loading="rendering" @click="downloadPoster">生成并下载 PNG</Button>
      </Space>
    </template>
    <input ref="fileInputRef" type="file" hidden accept="image/png,image/jpeg,image/webp" @change="handleFile" />
    <div class="poster-composer-layout">
      <div class="poster-preview-shell">
        <canvas ref="canvasRef" class="poster-canvas" @click="openFilePicker"></canvas>
        <Typography.Text type="secondary">点击预览也可以重新上传模板</Typography.Text>
      </div>
      <Form layout="vertical" class="poster-form">
        <Form.Item label="主标题">
          <Input v-model:value="headline" :maxlength="32" show-count />
        </Form.Item>
        <Form.Item label="副文案">
          <Input.TextArea v-model:value="subtitle" :rows="2" :maxlength="80" show-count />
        </Form.Item>
        <Form.Item label="行动按钮文案">
          <Input v-model:value="cta" :maxlength="24" />
        </Form.Item>
        <Form.Item label="邀请码">
          <Input v-model:value="inviteCode" :maxlength="32" addon-before="邀请码" />
        </Form.Item>
        <Form.Item label="二维码链接">
          <Input v-model:value="landingUrl" placeholder="留空则自动使用注册邀请链接" />
        </Form.Item>
        <Form.Item label="二维码尺寸">
          <Slider v-model:value="qrSize" :min="132" :max="220" :step="4" />
        </Form.Item>
        <Typography.Paragraph type="secondary" class="poster-hint">
          上传图片只在当前浏览器中处理，不会自动上传到后台。二维码默认绑定当前邀请码，适合直接分享给新用户。
        </Typography.Paragraph>
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
  cursor: pointer;
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
