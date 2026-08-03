<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';

import { Button, Result, Spin } from 'ant-design-vue';

import {
  createAdminThemeMessage,
  readAdminThemeSnapshot,
} from '#/utils/infinite-canvas-theme';

const configuredUrl = import.meta.env.VITE_INFINITE_CANVAS_URL?.trim();
const canvasUrl = computed(() =>
  configuredUrl || '/infinite-canvas/index.html#/canvas',
);

const frameKey = ref(0);
const frame = ref<HTMLIFrameElement>();
const loading = ref(true);
const failed = ref(false);
let themeObserver: MutationObserver | undefined;

function reload() {
  loading.value = true;
  failed.value = false;
  frameKey.value += 1;
}

function openStandalone() {
  window.open(canvasUrl.value, '_blank', 'noopener,noreferrer');
}

function syncTheme() {
  const target = frame.value?.contentWindow;
  if (!target) return;

  const targetOrigin = new URL(canvasUrl.value, window.location.href).origin;
  target.postMessage(
    createAdminThemeMessage(readAdminThemeSnapshot(document.documentElement)),
    targetOrigin,
  );
}

function handleFrameLoad() {
  loading.value = false;
  void nextTick(syncTheme);
}

onMounted(() => {
  themeObserver = new MutationObserver(syncTheme);
  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class', 'data-theme', 'style'],
  });
  themeObserver.observe(document.body, {
    attributes: true,
    attributeFilter: ['class', 'data-theme', 'style'],
  });
});

onBeforeUnmount(() => themeObserver?.disconnect());
</script>

<template>
  <div class="infinite-canvas-page">
    <div v-if="loading && !failed" class="canvas-state">
      <Spin size="large" tip="正在加载无限画布…" />
    </div>

    <Result
      v-if="failed"
      class="canvas-state"
      status="warning"
      title="无限画布加载失败"
      sub-title="请确认画布服务已启动，然后重新加载。"
    >
      <template #extra>
        <Button type="primary" @click="reload">重新加载</Button>
        <Button @click="openStandalone">独立打开</Button>
      </template>
    </Result>

    <iframe
      ref="frame"
      v-show="!failed"
      :key="frameKey"
      :src="canvasUrl"
      allow="clipboard-read; clipboard-write; fullscreen"
      class="canvas-frame"
      title="无限画布"
      @error="failed = true; loading = false"
      @load="handleFrameLoad"
    />
  </div>
</template>

<style scoped>
.infinite-canvas-page {
  position: relative;
  width: 100%;
  height: calc(100vh - 112px);
  min-height: 640px;
  overflow: hidden;
  background: hsl(var(--background));
  border-radius: 8px;
}

.canvas-frame {
  width: 100%;
  height: 100%;
  border: 0;
}

.canvas-state {
  position: absolute;
  inset: 0;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  background: hsl(var(--background));
}
</style>
