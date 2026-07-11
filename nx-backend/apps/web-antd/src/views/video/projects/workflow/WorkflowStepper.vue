<script setup lang="ts">
import type { WorkflowStepKey, WorkflowStepState } from '#/api/core/videoproject';

defineProps<{
  active: WorkflowStepKey;
  states: Record<WorkflowStepKey, WorkflowStepState>;
}>();

const emit = defineEmits<{ select: [step: WorkflowStepKey] }>();

const steps: Array<{ key: WorkflowStepKey; label: string }> = [
  { key: 'brief', label: '项目与剧本' },
  { key: 'assets', label: '角色与场景' },
  { key: 'storyboard', label: '分镜' },
  { key: 'generate', label: '生成' },
  { key: 'export', label: '导出' },
];
</script>

<template>
  <nav class="workflow-stepper" aria-label="视频制作步骤">
    <button
      v-for="(step, index) in steps"
      :key="step.key"
      type="button"
      class="workflow-step"
      :class="{ active: active === step.key, complete: states[step.key] === 'complete' }"
      :aria-current="active === step.key ? 'step' : undefined"
      @click="emit('select', step.key)"
    >
      <span class="step-index">{{ index + 1 }}</span>
      <span class="step-label">{{ step.label }}</span>
      <span class="step-state">{{ states[step.key] === 'complete' ? '已完成' : '' }}</span>
    </button>
  </nav>
</template>

<style scoped>
.workflow-stepper { display: grid; grid-template-columns: repeat(5, minmax(120px, 1fr)); overflow-x: auto; border-bottom: 1px solid #dbe3ee; background: #fff; }
.workflow-step { min-width: 120px; min-height: 64px; padding: 8px 12px; color: #475569; cursor: pointer; display: grid; grid-template-columns: 28px 1fr; grid-template-rows: 1fr 18px; gap: 0 8px; align-items: center; text-align: left; border: 0; border-bottom: 3px solid transparent; background: transparent; }
.workflow-step:hover { background: #f1f5f9; }
.workflow-step:focus-visible { outline: 3px solid rgba(37, 99, 235, 0.28); outline-offset: -3px; }
.workflow-step.active { color: #1d4ed8; border-bottom-color: #2563eb; background: #eff6ff; }
.step-index { grid-row: 1 / 3; width: 28px; height: 28px; display: inline-flex; align-items: center; justify-content: center; border: 1px solid #cbd5e1; border-radius: 50%; }
.complete .step-index { color: #065f46; border-color: #6ee7b7; background: #ecfdf5; }
.step-label { font-weight: 600; }
.step-state { min-height: 18px; color: #047857; font-size: 12px; }
</style>
