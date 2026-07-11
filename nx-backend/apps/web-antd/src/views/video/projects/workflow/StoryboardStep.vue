<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';

import { message } from 'ant-design-vue';

import {
  createShotApi,
  createShotsFromScriptApi,
  updateShotApi,
  type Project,
  type ScriptImportResult,
  type WorkflowShotStatus,
} from '#/api/core/videoproject';

import { splitScriptIntoShots } from './workflow';

const props = defineProps<{ project: Project; shots: WorkflowShotStatus[] }>();
const emit = defineEmits<{ changed: []; dirty: [value: boolean] }>();
const selectedId = ref('');
const importResult = ref<ScriptImportResult>();
const importing = ref(false);
const saving = ref(false);
const form = reactive({ actionDescription: '', aspectRatio: '16:9', characterIds: [] as string[], duration: 15, name: '', sceneId: '' });

const selected = computed(() => props.shots.find((item) => item.shot.id === selectedId.value));
const failed = computed(() => importResult.value?.failed || []);

watch(
  () => props.shots,
  (shots) => {
    if (!selectedId.value || !shots.some((item) => item.shot.id === selectedId.value)) selectedId.value = shots[0]?.shot.id || '';
  },
  { immediate: true },
);
watch(selected, (item) => {
  if (!item) return;
  Object.assign(form, {
    actionDescription: item.shot.actionDescription,
    aspectRatio: item.shot.aspectRatio,
    characterIds: [...item.shot.characterIds],
    duration: item.shot.duration,
    name: item.shot.name,
    sceneId: item.shot.sceneId,
  });
}, { immediate: true });

function markDirty() { emit('dirty', true); }

async function importParagraphs(items = splitScriptIntoShots(props.project.scriptContent)) {
  if (items.length === 0) {
    message.warning('剧本没有可导入的段落');
    return;
  }
  importing.value = true;
  try {
    importResult.value = await createShotsFromScriptApi(props.project.id, { items, scriptRevision: props.project.scriptRevision });
    emit('changed');
  } finally { importing.value = false; }
}

async function retryFailed() {
  const source = splitScriptIntoShots(props.project.scriptContent);
  const indexes = new Set(failed.value.map((item) => item.index));
  await importParagraphs(source.filter((item) => indexes.has(item.index)));
}

async function addManualShot() {
  await createShotApi(props.project.id, { actionDescription: '请填写动作描述', aspectRatio: '16:9', duration: 15, name: `分镜 ${props.shots.length + 1}` });
  emit('changed');
}

async function save() {
  if (!selected.value) return;
  if (!form.actionDescription.trim()) throw new Error('请填写动作描述');
  saving.value = true;
  try {
    const shot = selected.value.shot;
    await updateShotApi(shot.id, { ...shot, ...form });
    emit('dirty', false);
    emit('changed');
    message.success('分镜已保存');
  } finally { saving.value = false; }
}

defineExpose({ save });
</script>

<template>
  <div class="storyboard-step">
    <div v-if="shots.length === 0" class="storyboard-empty">
      <h3>还没有分镜</h3><p>可以按剧本段落批量创建，也可以手动添加。</p>
      <div><a-button :loading="importing" @click="importParagraphs()">从剧本创建分镜</a-button><a-button @click="addManualShot">手动添加</a-button></div>
    </div>
    <template v-else>
      <div class="storyboard-toolbar">
        <a-button :loading="importing" @click="importParagraphs()">从剧本创建分镜</a-button>
        <a-button @click="addManualShot">手动添加</a-button>
      </div>
      <select v-model="selectedId" class="mobile-shot-selector" aria-label="选择分镜">
        <option v-for="item in shots" :key="item.shot.id" :value="item.shot.id">{{ item.shot.name || `分镜 ${item.shot.orderNum}` }} · {{ item.readiness }}</option>
      </select>
      <div class="storyboard-layout">
        <nav class="shot-nav" aria-label="分镜列表">
          <button v-for="item in shots" :key="item.shot.id" type="button" :class="{ active: item.shot.id === selectedId }" @click="selectedId = item.shot.id">
            <span>{{ item.shot.orderNum }}</span><strong>{{ item.shot.name || '未命名分镜' }}</strong><small>{{ item.readiness }}</small>
          </button>
        </nav>
        <section v-if="selected" class="shot-editor">
          <label>名称<a-input v-model:value="form.name" @change="markDirty" /></label>
          <label>动作描述<a-textarea v-model:value="form.actionDescription" :rows="5" @change="markDirty" /></label>
          <div class="editor-grid">
            <label>角色<a-select v-model:value="form.characterIds" mode="tags" @change="markDirty" /></label>
            <label>场景<a-input v-model:value="form.sceneId" @change="markDirty" /></label>
            <label>时长<a-select v-model:value="form.duration" :options="[{value:5,label:'5 秒'},{value:10,label:'10 秒'},{value:15,label:'15 秒'}]" @change="markDirty" /></label>
            <label>画幅<a-select v-model:value="form.aspectRatio" :options="[{value:'16:9',label:'16:9'},{value:'9:16',label:'9:16'},{value:'1:1',label:'1:1'}]" @change="markDirty" /></label>
          </div>
          <p v-if="!form.actionDescription.trim()" role="alert" class="field-error">请填写动作描述</p>
        </section>
      </div>
    </template>
    <section v-if="importResult" class="import-result" aria-live="polite">
      <span>created {{ importResult.created.length }}</span><span>existing {{ importResult.existing.length }}</span><span>failed {{ importResult.failed.length }}</span>
      <a-button v-if="failed.length" @click="retryFailed">重试失败项</a-button>
    </section>
  </div>
</template>

<style scoped>
.storyboard-step { display:grid; gap:16px; }.storyboard-empty { min-height:300px; display:flex; flex-direction:column; align-items:center; justify-content:center; text-align:center; }.storyboard-empty div,.storyboard-toolbar,.import-result { display:flex; flex-wrap:wrap; gap:10px; }.storyboard-layout { display:grid; grid-template-columns:260px minmax(0,1fr); gap:20px; }.shot-nav { display:grid; align-content:start; gap:6px; }.shot-nav button { min-height:56px; padding:8px; display:grid; grid-template-columns:28px 1fr; text-align:left; border:1px solid #dbe3ee; border-radius:6px; background:#fff; }.shot-nav button.active { border-color:#2563eb; background:#eff6ff; }.shot-nav span { grid-row:1/3; }.shot-nav small { color:#64748b; }.shot-editor,label { display:grid; gap:8px; }.shot-editor { gap:16px; }.editor-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:12px; }.mobile-shot-selector { display:none; width:100%; min-height:44px; }.field-error { color:#b91c1c; }:deep(.ant-input),:deep(.ant-select-selector),:deep(.ant-btn) { min-height:44px; }
@media(max-width:720px){.mobile-shot-selector{display:block}.shot-nav{display:none}.storyboard-layout{grid-template-columns:1fr}.editor-grid{grid-template-columns:1fr}}
</style>
