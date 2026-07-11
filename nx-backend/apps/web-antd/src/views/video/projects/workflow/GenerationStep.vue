<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue';

import {
  Button as AButton,
  Input as AInput,
  message,
} from 'ant-design-vue';

import {
  batchGenerateShotsSafeApi,
  generateShotSafeApi,
  getProjectWorkflowApi,
  reconcileGenerationSubmissionApi,
  type WorkflowShotStatus,
} from '#/api/core/videoproject';

import { groupGeneratableShots, readinessFilter, type ReadinessFilter } from './workflow';
import { createWorkflowPollingController } from './useWorkflowPolling';
import VersionDrawer from './VersionDrawer.vue';

const props = defineProps<{ projectId: string; shots: WorkflowShotStatus[] }>();
const emit = defineEmits<{ changed: [] }>();
const filters: Array<{ key: ReadinessFilter; label: string }> = [
  { key: 'generatable', label: '可生成' }, { key: 'incomplete', label: '待完善' },
  { key: 'generating', label: '生成中' }, { key: 'completed', label: '已完成' },
];
const activeFilter = ref<ReadinessFilter>('generatable');
const busy = ref(false);
const busyShots = ref(new Set<string>());
const requestKeys = ref(new Map<string, string>());
const recoveryTaskIds = reactive<Record<string, string>>({});
const drawerOpen = ref(false);
const drawerShotId = ref('');
let drawerTriggerShotId = '';
const timeoutShots = ref(new Set<string>());
const resumedPolling = new Set<string>();
const polling = createWorkflowPollingController({ delay: 4000, maxAttempts: 30 });

const visibleShots = computed(() => props.shots.filter((item) => readinessFilter(item.readiness) === activeFilter.value));
const counts = computed(() => Object.fromEntries(filters.map((filter) => [filter.key, props.shots.filter((item) => readinessFilter(item.readiness) === filter.key).length])));
const groups = computed(() => groupGeneratableShots(props.shots));
const drawerShot = computed(() => props.shots.find((item) => item.shot.id === drawerShotId.value)?.shot);

function newRequestKey() { return crypto.randomUUID(); }
function requestKeyFor(shotId: string, forceNew = false) {
  if (forceNew || !requestKeys.value.has(shotId)) {
    requestKeys.value.set(shotId, newRequestKey());
    requestKeys.value = new Map(requestKeys.value);
  }
  return requestKeys.value.get(shotId)!;
}
function clearTerminalKey(shotId: string, status: string) {
  if (!['cancelled', 'completed', 'failed'].includes(status)) return;
  requestKeys.value.delete(shotId);
  requestKeys.value = new Map(requestKeys.value);
}

function startPolling(shotId: string) {
  resumedPolling.add(shotId);
  polling.start(
    shotId,
    async () => {
      const next = await getProjectWorkflowApi(props.projectId);
      const item = next.shots.find((shot) => shot.shot.id === shotId);
      emit('changed');
      if (!item) return 'failed';
      if (item.readiness === 'completed') return 'completed';
      if (item.readiness === 'failed') return 'failed';
      if (item.readiness === 'recovery') return 'unknown_outcome';
      return 'accepted';
    },
    (status) => {
      resumedPolling.delete(shotId);
      clearTerminalKey(shotId, status);
    },
    () => {
      resumedPolling.delete(shotId);
      timeoutShots.value.add(shotId);
      timeoutShots.value = new Set(timeoutShots.value);
    },
  );
}

watch(
  () => props.shots,
  (shots) => {
    for (const item of shots) {
      if (!item.activeSubmission) continue;
      requestKeys.value.set(item.shot.id, item.activeSubmission.requestKey);
      if (item.activeSubmission.taskId) recoveryTaskIds[item.shot.id] = item.activeSubmission.taskId;
      if (item.activeSubmission.status === 'unknown_outcome') continue;
      if (item.readiness === 'generating' && !resumedPolling.has(item.shot.id)) startPolling(item.shot.id);
    }
    requestKeys.value = new Map(requestKeys.value);
    if (!drawerOpen.value && drawerTriggerShotId) void restoreDrawerFocus();
  },
  { immediate: true },
);
watch(drawerOpen, (open) => { if (!open && drawerTriggerShotId) setTimeout(restoreDrawerFocus, 0); });

async function generateOne(item: WorkflowShotStatus, forceNew = false) {
  if (busyShots.value.has(item.shot.id) || !item.canGenerate) return;
  busyShots.value.add(item.shot.id); busyShots.value = new Set(busyShots.value);
  try {
    const requestKey = requestKeyFor(item.shot.id, forceNew);
    const result = await generateShotSafeApi(item.shot.id, { requestKey }) as unknown as { status?: string };
    if (result.status === 'unknown_outcome') message.warning('提交结果未知，请使用检查结果或对账，不要重复提交');
    else message.success('已提交生成任务');
    startPolling(item.shot.id);
    emit('changed');
  } finally { busyShots.value.delete(item.shot.id); busyShots.value = new Set(busyShots.value); }
}

async function generateAll() {
  const eligible = props.shots.filter((item) => item.canGenerate);
  if (eligible.length === 0 || busy.value) return;
  busy.value = true;
  try {
    await batchGenerateShotsSafeApi(props.projectId, { items: eligible.map((item) => ({ shotId: item.shot.id, requestKey: requestKeyFor(item.shot.id) })) });
    eligible.forEach((item) => startPolling(item.shot.id));
    message.success(`已提交 ${eligible.length} 个分镜`);
    emit('changed');
  } finally { busy.value = false; }
}

async function reconcile(item: WorkflowShotStatus) {
  const requestKey = requestKeys.value.get(item.shot.id);
  const taskId = recoveryTaskIds[item.shot.id]?.trim();
  if (!requestKey || !taskId) { message.warning('请输入上游 task ID'); return; }
  await reconcileGenerationSubmissionApi(requestKey, { taskId });
  startPolling(item.shot.id);
  message.success('已完成对账，继续检查生成状态');
}

function inspectRecovery() { emit('changed'); message.info('已刷新提交状态'); }
function manualRefresh(shotId: string) {
  timeoutShots.value.delete(shotId);
  timeoutShots.value = new Set(timeoutShots.value);
  startPolling(shotId);
}
function openVersions(item: WorkflowShotStatus) { drawerTriggerShotId = item.shot.id; drawerShotId.value = item.shot.id; drawerOpen.value = true; }
function handleVersionSelected() { activeFilter.value = 'completed'; emit('changed'); }
async function restoreDrawerFocus() {
  await nextTick();
  let attempts = 0;
  const focus = () => {
    const trigger = document.querySelector<HTMLElement>(`[data-shot-id="${drawerTriggerShotId}"] [data-version-trigger]`);
    trigger?.focus();
    attempts += 1;
    if (document.activeElement !== trigger && attempts < 5) requestAnimationFrame(focus);
  };
  requestAnimationFrame(focus);
}
defineExpose({ generateAll });
onBeforeUnmount(() => polling.stopAll());
</script>

<template>
  <div class="generation-step">
    <div class="filter-bar" role="tablist" aria-label="分镜生成状态">
      <button v-for="filter in filters" :key="filter.key" type="button" :class="{ active: activeFilter === filter.key }" @click="activeFilter = filter.key">{{ filter.label }} <strong>{{ counts[filter.key] }}</strong></button>
    </div>
    <div class="generation-summary">可生成 {{ groups.total }}：新分镜 {{ groups.ready }}、内容变化 {{ groups.stale }}、上次失败 {{ groups.failed }}</div>
    <details><summary>高级设置</summary><p>模型、分辨率和参考模式在分镜步骤中按镜头保存。</p></details>
    <div class="generation-list">
      <article v-for="item in visibleShots" :key="item.shot.id" :data-shot-id="item.shot.id">
        <div><strong>{{ item.shot.name || `分镜 ${item.shot.orderNum}` }}</strong><span>{{ item.readiness }}</span></div>
        <p>{{ item.shot.actionDescription || '缺少动作描述，请返回分镜修改' }}</p>
        <p v-if="timeoutShots.has(item.shot.id)" class="notice">仍在处理中，可<a-button type="link" @click="manualRefresh(item.shot.id)">手动刷新</a-button></p>
        <div v-if="item.readiness === 'recovery'" class="recovery-actions">
          <a-input v-model:value="recoveryTaskIds[item.shot.id]" placeholder="上游 task ID" />
          <a-button @click="inspectRecovery">检查结果</a-button><a-button @click="reconcile(item)">对账</a-button>
        </div>
        <div v-else class="shot-actions">
          <a-button v-if="item.canGenerate" :loading="busyShots.has(item.shot.id)" @click="generateOne(item)">生成</a-button>
          <a-button v-if="item.readiness === 'completed'" @click="generateOne(item, true)">再生成一个版本</a-button>
          <a-button data-version-trigger @click="openVersions(item)">查看版本</a-button>
        </div>
      </article>
    </div>
    <VersionDrawer v-model:open="drawerOpen" :shot="drawerShot" @closed="restoreDrawerFocus" @selected="handleVersionSelected" />
  </div>
</template>

<style scoped>
.generation-step { display:grid; gap:16px; }.filter-bar { display:grid; grid-template-columns:repeat(4,minmax(110px,1fr)); overflow-x:auto; border-bottom:1px solid #dbe3ee; }.filter-bar button { min-height:48px; color:#475569; cursor:pointer; border:0; border-bottom:3px solid transparent; background:#fff; }.filter-bar button.active { color:#1d4ed8; border-bottom-color:#2563eb; background:#eff6ff; }.generation-summary,.notice { color:#64748b; }.generation-list { display:grid; gap:10px; }.generation-list article { padding:16px; display:grid; grid-template-columns:minmax(180px,.7fr) minmax(220px,1.4fr) auto; gap:12px; align-items:center; border:1px solid #dbe3ee; border-radius:8px; background:#fff; }.generation-list article>div:first-child { display:grid; }.generation-list span { color:#64748b; }.shot-actions,.recovery-actions { display:flex; align-items:center; justify-content:flex-end; flex-wrap:wrap; gap:8px; }.recovery-actions :deep(.ant-input){width:180px}:deep(.ant-btn),:deep(.ant-input),summary { min-height:44px; }
@media(max-width:760px){.generation-list article{grid-template-columns:1fr}.shot-actions,.recovery-actions{justify-content:flex-start}.filter-bar{grid-template-columns:repeat(4,120px)}}
</style>
