<script setup lang="ts">
import { ref, watch } from 'vue';

import {
  Button as AButton,
  Drawer as ADrawer,
  Empty as AEmpty,
  Spin as ASpin,
  message,
} from 'ant-design-vue';

import {
  listShotVideoVersionsApi,
  setShotVideoVersionApi,
  type Shot,
  type ShotVideoVersion,
} from '#/api/core/videoproject';

const props = defineProps<{ open: boolean; shot?: Shot }>();
const emit = defineEmits<{ closed: []; selected: []; 'update:open': [value: boolean] }>();
const versions = ref<ShotVideoVersion[]>([]);
const loading = ref(false);
const selecting = ref('');

async function load() {
  if (!props.open || !props.shot) return;
  loading.value = true;
  try { versions.value = await listShotVideoVersionsApi(props.shot.id); }
  finally { loading.value = false; }
}

async function selectVersion(version: ShotVideoVersion) {
  if (!props.shot || version.isCurrent) return;
  selecting.value = version.id;
  try {
    await setShotVideoVersionApi(props.shot.id, version.id);
    message.success('已设为当前版本');
    await load();
    emit('selected');
  } finally { selecting.value = ''; }
}

watch(() => [props.open, props.shot?.id], load, { immediate: true });
function afterOpenChange(open: boolean) { if (!open) emit('closed'); }
</script>

<template>
  <a-drawer :open="open" width="480" title="视频版本" @after-open-change="afterOpenChange" @close="emit('update:open', false)">
    <a-spin :spinning="loading">
      <div class="version-list">
        <article v-for="version in versions" :key="version.id" :class="{ current: version.isCurrent }">
          <video v-if="version.videoUrl" :src="version.videoUrl" controls preload="metadata" />
          <div class="version-meta"><strong>{{ version.isCurrent ? '当前版本' : `版本 ${version.id}` }}</strong><span>{{ version.status }} · {{ version.shotRevision }}</span></div>
          <a-button :disabled="version.isCurrent || !version.videoUrl" :loading="selecting === version.id" @click="selectVersion(version)">设为当前</a-button>
        </article>
        <a-empty v-if="!loading && versions.length === 0" description="暂无视频版本" />
      </div>
    </a-spin>
  </a-drawer>
</template>

<style scoped>
.version-list { display:grid; gap:12px; }.version-list article { padding:12px; display:grid; grid-template-columns:140px 1fr auto; gap:12px; align-items:center; border:1px solid #dbe3ee; border-radius:8px; }.version-list article.current { border-color:#34d399; background:#ecfdf5; }.version-list video { width:140px; aspect-ratio:16/9; object-fit:cover; background:#0f172a; }.version-meta { min-width:0; display:grid; }.version-meta span { color:#64748b; }:deep(.ant-btn) { min-height:44px; }
@media(max-width:560px){.version-list article{grid-template-columns:1fr}.version-list video{width:100%}}
</style>
