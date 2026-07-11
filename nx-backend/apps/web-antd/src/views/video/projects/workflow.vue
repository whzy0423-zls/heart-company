<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';

import { message } from 'ant-design-vue';

import {
  getProjectWorkflowApi,
  type ProjectWorkflow,
  type WorkflowStepKey,
} from '#/api/core/videoproject';

import WorkflowStepper from './workflow/WorkflowStepper.vue';
import { normalizeWorkflowStep, workflowSteps } from './workflow/workflow';
import { createWorkflowNavigationController } from './workflow/useWorkflowNavigation';

const route = useRoute();
const router = useRouter();
const workflow = ref<ProjectWorkflow>();
const loading = ref(true);
const fatalError = ref('');
const primaryBusy = ref(false);
const dirtyDialogVisible = ref(false);
const navigation = createWorkflowNavigationController();

const projectId = computed(() => String(route.params.id || ''));
const activeStep = ref<WorkflowStepKey>('brief');
const stepIndex = computed(() => workflowSteps.indexOf(activeStep.value));
const advancedPath = computed(() => `/video/projects/${projectId.value}/workbench/advanced`);
const currentState = computed(() => workflow.value?.steps[activeStep.value] || 'blocked');
const primaryLabel = computed(() => {
  if (activeStep.value === 'export') return '开始合成';
  if (activeStep.value === 'generate') return '生成可用分镜';
  return stepIndex.value === workflowSteps.length - 1 ? '完成' : '保存并继续';
});

async function loadWorkflow() {
  loading.value = true;
  fatalError.value = '';
  try {
    workflow.value = await getProjectWorkflowApi(projectId.value);
    const queryStep = Array.isArray(route.query.step) ? route.query.step[0] : route.query.step;
    activeStep.value = normalizeWorkflowStep(queryStep, workflow.value.recommendedStep);
    if (queryStep !== activeStep.value) {
      await router.replace({ query: { ...route.query, step: activeStep.value } });
    }
  } catch (error) {
    fatalError.value = error instanceof Error ? error.message : '工作台加载失败';
  } finally {
    loading.value = false;
  }
}

const retryLoad = () => loadWorkflow();

async function performStepChange(step: WorkflowStepKey) {
  activeStep.value = step;
  await router.replace({ query: { ...route.query, step } });
}

async function selectStep(step: WorkflowStepKey) {
  await navigation.request(() => performStepChange(step));
  dirtyDialogVisible.value = navigation.pending();
}

async function runPrimaryAction() {
  if (primaryBusy.value) return;
  primaryBusy.value = true;
  try {
    if (activeStep.value === 'generate' || activeStep.value === 'export') {
      message.info(currentState.value === 'blocked' ? '请先处理页面中标出的缺项' : primaryLabel.value);
      return;
    }
    const next = workflowSteps[Math.min(stepIndex.value + 1, workflowSteps.length - 1)] ?? activeStep.value;
    await performStepChange(next);
  } finally {
    primaryBusy.value = false;
  }
}

function beforeUnload(event: BeforeUnloadEvent) {
  if (!navigation.isDirty()) return;
  event.preventDefault();
  event.returnValue = '';
}

async function saveAndContinue() {
  await navigation.saveAndContinue(async () => undefined);
  dirtyDialogVisible.value = false;
}

async function discardAndContinue() {
  await navigation.discardAndContinue(loadWorkflow);
  dirtyDialogVisible.value = false;
}

function cancelNavigation() {
  navigation.cancel();
  dirtyDialogVisible.value = false;
}

watch(projectId, loadWorkflow);
onMounted(() => {
  window.addEventListener('beforeunload', beforeUnload);
  void loadWorkflow();
});
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));
</script>

<template>
  <div class="workflow-page">
    <header class="workflow-header">
      <div>
        <a-breadcrumb>
          <a-breadcrumb-item><router-link to="/video/production">视频制作</router-link></a-breadcrumb-item>
          <a-breadcrumb-item>{{ workflow?.project.name || '项目工作台' }}</a-breadcrumb-item>
        </a-breadcrumb>
        <h1>{{ workflow?.project.name || '项目工作台' }}</h1>
      </div>
      <router-link class="advanced-link" :to="advancedPath">高级工作台</router-link>
    </header>

    <WorkflowStepper
      v-if="workflow"
      :active="activeStep"
      :states="workflow.steps"
      @select="selectStep"
    />

    <main class="workflow-main">
      <div v-if="loading" class="workflow-loading" aria-live="polite">
        <a-skeleton active :paragraph="{ rows: 7 }" />
      </div>
      <div v-else-if="fatalError" class="workflow-fatal" role="alert">
        <h2>暂时无法打开工作台</h2>
        <p>{{ fatalError }}</p>
        <a-button type="primary" @click="retryLoad">重试</a-button>
      </div>
      <section v-else-if="workflow" class="workflow-panel" :data-step="activeStep">
        <p class="step-eyebrow">步骤 {{ stepIndex + 1 }} / 5</p>
        <h2>{{ activeStep === 'brief' ? '先把故事讲清楚' : activeStep === 'assets' ? '准备会重复使用的角色与场景' : activeStep === 'storyboard' ? '把剧本变成可执行分镜' : activeStep === 'generate' ? '只生成准备完成的镜头' : '检查版本并导出成片' }}</h2>
        <p aria-live="polite" class="step-status">
          {{ currentState === 'blocked' ? '当前步骤还有内容需要处理，你仍可查看并返回修改。' : '当前步骤可以继续。' }}
        </p>
        <div class="step-placeholder">
          <span v-if="activeStep === 'brief'">项目设置与剧本将在这里编辑。</span>
          <span v-else-if="activeStep === 'assets'">角色和场景将在这里管理。</span>
          <span v-else-if="activeStep === 'storyboard'">分镜导航与编辑器将在这里显示。</span>
          <span v-else-if="activeStep === 'generate'">生成检查、状态筛选与版本将在这里显示。</span>
          <span v-else>合成设置、进度和成片将在这里显示。</span>
        </div>
      </section>
    </main>

    <footer class="workflow-footer">
      <span>{{ workflow?.project.updateTime ? `最近更新 ${workflow.project.updateTime}` : '' }}</span>
      <a-button
        data-primary-action
        type="primary"
        :loading="primaryBusy"
        :disabled="loading || !!fatalError"
        @click="runPrimaryAction"
      >
        {{ primaryLabel }}
      </a-button>
    </footer>

    <a-modal :open="dirtyDialogVisible" title="保存当前修改？" :footer="null" @cancel="cancelNavigation">
      <p>当前内容尚未保存。保存后继续，或放弃本次修改。</p>
      <div class="dirty-actions">
        <a-button @click="cancelNavigation">取消</a-button>
        <a-button @click="discardAndContinue">放弃修改</a-button>
        <a-button type="primary" @click="saveAndContinue">保存并继续</a-button>
      </div>
    </a-modal>
  </div>
</template>

<style scoped>
.workflow-page { min-height: 100dvh; color: #1e293b; background: #f8fafc; }
.workflow-header { min-height: 88px; padding: 16px 24px; display: flex; align-items: center; justify-content: space-between; gap: 16px; border-bottom: 1px solid #dbe3ee; background: #fff; }
.workflow-header h1 { margin: 8px 0 0; font-size: 24px; letter-spacing: 0; }
.advanced-link { min-height: 44px; padding: 0 12px; display: inline-flex; align-items: center; color: #1d4ed8; }
.workflow-main { width: min(1180px, 100%); min-height: calc(100dvh - 236px); margin: 0 auto; padding: 24px 24px 112px; }
.workflow-loading, .workflow-fatal, .workflow-panel { min-height: 420px; }
.workflow-fatal { display: flex; flex-direction: column; align-items: flex-start; justify-content: center; }
.workflow-panel h2 { margin: 4px 0 8px; font-size: 22px; letter-spacing: 0; }
.step-eyebrow { margin: 0; color: #2563eb; font-weight: 600; }
.step-status { color: #64748b; }
.step-placeholder { min-height: 300px; padding: 32px 0; color: #475569; border-top: 1px solid #e2e8f0; }
.workflow-footer { position: fixed; right: 0; bottom: 0; left: 0; z-index: 20; min-height: calc(72px + env(safe-area-inset-bottom)); padding: 12px 24px calc(12px + env(safe-area-inset-bottom)); display: flex; align-items: center; justify-content: space-between; gap: 16px; border-top: 1px solid #cbd5e1; background: rgba(255, 255, 255, 0.97); }
.workflow-footer :deep(.ant-btn) { min-width: 160px; min-height: 44px; border-radius: 6px; }
.dirty-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 8px; }
@media (max-width: 640px) { .workflow-header { padding: 12px 16px; align-items: flex-start; } .workflow-header h1 { font-size: 20px; } .workflow-main { padding: 20px 16px 128px; } .workflow-footer { align-items: stretch; flex-direction: column; padding-inline: 16px; } .workflow-footer :deep(.ant-btn) { width: 100%; } }
@media (prefers-reduced-motion: reduce) { *, *::before, *::after { scroll-behavior: auto !important; transition-duration: 0s !important; animation: none !important; } }
</style>
