<script setup lang="ts">
import { computed, reactive, watch } from 'vue';

import {
  Input as AInput,
  Textarea as ATextarea,
  message,
} from 'ant-design-vue';

import { updateProjectApi, type Project } from '#/api/core/videoproject';

import { splitScriptIntoShots } from './workflow';

const props = defineProps<{ project: Project }>();
const emit = defineEmits<{ dirty: [value: boolean]; saved: [] }>();

const form = reactive({ name: '', theme: '', scriptContent: '', styleGuide: '' });
const error = reactive({ scriptContent: '' });

watch(
  () => props.project,
  (project) => Object.assign(form, {
    name: project.name,
    scriptContent: project.scriptContent || '',
    styleGuide: project.styleGuide || '',
    theme: project.theme || '',
  }),
  { immediate: true },
);

const scriptCharacterCount = computed(() => [...form.scriptContent].length);
const estimatedParagraphCount = computed(() => splitScriptIntoShots(form.scriptContent).length);

function markDirty() {
  error.scriptContent = '';
  emit('dirty', true);
}

async function save() {
  if (!form.scriptContent.trim()) {
    error.scriptContent = '请先填写完整剧本；已有旧分镜的项目可以直接进入分镜步骤。';
    throw new Error(error.scriptContent);
  }
  await updateProjectApi(props.project.id, { ...props.project, ...form });
  emit('dirty', false);
  emit('saved');
  message.success('项目与剧本已保存');
}

defineExpose({ save });
</script>

<template>
  <div class="brief-step">
    <div class="field-grid">
      <label>项目名称<a-input v-model:value="form.name" placeholder="请输入项目名称" @change="markDirty" /></label>
      <label>主题<a-input v-model:value="form.theme" placeholder="请输入项目主题" @change="markDirty" /></label>
    </div>
    <label>
      完整剧本
      <a-textarea v-model:value="form.scriptContent" :rows="14" placeholder="请粘贴或输入完整剧本" @change="markDirty" />
    </label>
    <p v-if="error.scriptContent" role="alert" class="field-error">{{ error.scriptContent }}</p>
    <div class="script-summary">
      <span>{{ scriptCharacterCount }} 字</span>
      <span>预计 {{ estimatedParagraphCount }} 个分镜段落</span>
    </div>
    <label>视觉风格<a-textarea v-model:value="form.styleGuide" :rows="3" placeholder="请输入视觉风格与画面要求" @change="markDirty" /></label>
  </div>
</template>

<style scoped>
.brief-step { display: grid; gap: 20px; }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
label { display: grid; gap: 8px; color: #334155; font-weight: 600; }
:deep(.ant-input), :deep(.ant-input-affix-wrapper), :deep(textarea) { min-height: 44px; font-weight: 400; }
.script-summary { display: flex; flex-wrap: wrap; gap: 16px; color: #64748b; }
.field-error { margin: -12px 0 0; color: #b91c1c; }
@media (max-width: 640px) { .field-grid { grid-template-columns: 1fr; } }
</style>
