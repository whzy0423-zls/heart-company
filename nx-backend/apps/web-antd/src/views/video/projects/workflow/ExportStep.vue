<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref } from 'vue';

import { message } from 'ant-design-vue';

import {
  composeProjectSafeApi,
  getComposeJobApi,
  type ComposeVideoResponse,
  type Project,
  type WorkflowShotStatus,
} from '#/api/core/videoproject';

const props = defineProps<{ current: boolean; project: Project; shots: WorkflowShotStatus[] }>();
const emit = defineEmits<{ changed: [] }>();
const excluded = ref(new Set<string>());
const options = reactive({ enableSubtitles: false, musicUrl: '', transition: 'none' });
const partialAcknowledged = ref(false);
const busy = ref(false);
const job = ref<ComposeVideoResponse>();
let timer: ReturnType<typeof setTimeout> | undefined;

const excludedShotIds = computed(() => [...excluded.value].sort());
const includedShotIds = computed(() => props.shots.map((item) => item.shot.id).filter((id) => !excluded.value.has(id)));
const incomplete = computed(() => props.shots.filter((item) => item.readiness !== 'completed'));

function toggleShot(shotId: string, include: boolean) {
  if (include) excluded.value.delete(shotId); else excluded.value.add(shotId);
  excluded.value = new Set(excluded.value);
  partialAcknowledged.value = false;
}

function stopPolling() { if (timer) clearTimeout(timer); timer = undefined; }
async function pollJob() {
  if (!job.value?.jobId) return;
  const next = await getComposeJobApi(props.project.id, job.value.jobId);
  job.value = next;
  if (next.status === 'completed') { busy.value = false; emit('changed'); message.success('成片已生成'); return; }
  if (next.status === 'failed') { busy.value = false; message.error(next.error || '合成失败，可保留设置后重试'); return; }
  timer = setTimeout(pollJob, 2000);
}

async function compose() {
  if (busy.value) return;
  if (includedShotIds.value.length === 0) { message.warning('至少保留一个分镜'); return; }
  if (excludedShotIds.value.length > 0 && !partialAcknowledged.value) { message.warning('请确认本次部分合成的排除清单'); return; }
  busy.value = true;
  stopPolling();
  try {
    job.value = await composeProjectSafeApi(props.project.id, {
      ...options,
      excludedShotIds: excludedShotIds.value,
      partialAcknowledged: partialAcknowledged.value,
    });
    partialAcknowledged.value = false;
    await pollJob();
  } catch (error) { busy.value = false; throw error; }
}

async function copyLink() {
  if (!props.project.finalVideoUrl) return;
  await navigator.clipboard.writeText(props.project.finalVideoUrl);
  message.success('链接已复制');
}
function downloadVideo() {
  if (!props.project.finalVideoUrl) return;
  const link = document.createElement('a'); link.href = props.project.finalVideoUrl; link.download = `${props.project.name || 'video'}.mp4`; link.click();
  message.success('已开始下载');
}

defineExpose({ compose });
onBeforeUnmount(stopPolling);
</script>

<template>
  <div class="export-step">
    <section class="participation">
      <div><h3>合成清单</h3><p>纳入 {{ includedShotIds.length }} 个，排除 {{ excludedShotIds.length }} 个</p></div>
      <div class="shot-checks">
        <label v-for="item in shots" :key="item.shot.id">
          <a-checkbox :checked="!excluded.has(item.shot.id)" @change="(event: any) => toggleShot(item.shot.id, event.target.checked)" />
          <span>{{ item.shot.name || `分镜 ${item.shot.orderNum}` }}</span><small>{{ item.readiness }}</small>
        </label>
      </div>
      <p v-if="incomplete.length" class="warning">未完成镜头默认会阻止全量合成；如需排除，请逐项取消勾选。</p>
      <label v-if="excludedShotIds.length" class="ack"><a-checkbox v-model:checked="partialAcknowledged" />我确认本次排除：{{ excludedShotIds.join('、') }}</label>
    </section>
    <section class="compose-options">
      <label>转场<a-select v-model:value="options.transition" :options="[{value:'none',label:'无'},{value:'fade',label:'淡入淡出'}]" /></label>
      <label>背景音乐 URL<a-input v-model:value="options.musicUrl" /></label>
      <label><a-switch v-model:checked="options.enableSubtitles" /> 添加字幕</label>
    </section>
    <section v-if="job" class="job-status" aria-live="polite">
      <strong>{{ job.status }}</strong><a-progress :percent="job.progress" /><span v-if="job.error" role="alert">{{ job.error }}</span>
      <a-button v-if="job.status === 'failed'" @click="compose">重试合成</a-button>
    </section>
    <section class="final-result">
      <div><h3>{{ current ? '当前成片' : project.finalVideoUrl ? '内容已变化，需要重新合成' : '尚未合成' }}</h3><p v-if="!current && project.finalVideoUrl">旧成片仍可预览和下载。</p></div>
      <video v-if="project.finalVideoUrl" :src="project.finalVideoUrl" controls preload="metadata" />
      <div v-if="project.finalVideoUrl" class="result-actions"><a-button @click="copyLink">复制链接</a-button><a-button @click="downloadVideo">下载成片</a-button></div>
    </section>
  </div>
</template>

<style scoped>
.export-step { display:grid; gap:24px; }.participation,.compose-options,.job-status,.final-result { display:grid; gap:14px; }.shot-checks { display:grid; grid-template-columns:repeat(auto-fill,minmax(240px,1fr)); gap:8px; }.shot-checks label { min-height:48px; padding:8px 10px; display:grid; grid-template-columns:auto 1fr; gap:0 8px; align-items:center; border:1px solid #dbe3ee; border-radius:6px; background:#fff; }.shot-checks small { grid-column:2; color:#64748b; }.warning { color:#92400e; }.ack { min-height:44px; display:flex; align-items:center; gap:8px; }.compose-options { grid-template-columns:180px minmax(240px,1fr) auto; align-items:end; }.compose-options label { display:grid; gap:8px; }.final-result video { width:min(760px,100%); max-height:440px; background:#0f172a; }.result-actions { display:flex; gap:8px; }:deep(.ant-btn),:deep(.ant-input),:deep(.ant-select-selector) { min-height:44px; }
@media(max-width:720px){.compose-options{grid-template-columns:1fr}}
</style>
