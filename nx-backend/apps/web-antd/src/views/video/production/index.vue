<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';

import {
  Button as AButton,
  Progress as AProgress,
  Skeleton as ASkeleton,
  message,
} from 'ant-design-vue';

import { createProjectApi, listProjectsApi, type Project } from '#/api/core/videoproject';

const router = useRouter();
const projects = ref<Project[]>([]);
const loading = ref(true);
const loadError = ref('');
const creating = ref(false);

async function load() {
  loading.value = true;
  loadError.value = '';
  try {
    const result = await listProjectsApi({ page: 1, pageSize: 6 });
    projects.value = result.items;
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : '项目加载失败';
  } finally { loading.value = false; }
}

const retryLoad = () => load();

async function createProject() {
  if (creating.value) return;
  creating.value = true;
  try {
    const created = await createProjectApi({ name: `未命名项目 ${new Date().toLocaleDateString()}` });
    await router.push(`/video/projects/${created.id}/workbench`);
  } catch (error) {
    message.error('新建项目失败，请重试');
  } finally { creating.value = false; }
}

function continueProject(project: Project) {
  router.push(`/video/projects/${project.id}/workbench`);
}

onMounted(load);
</script>

<template>
  <div class="production-page">
    <header class="production-header">
      <div><h1>制片工作台</h1><p>继续最近项目，或开始一个新的五步制作流程。</p></div>
      <a-button type="primary" :loading="creating" :disabled="creating" @click="createProject">新建项目</a-button>
    </header>
    <main>
      <div v-if="loading" aria-live="polite"><a-skeleton active :paragraph="{ rows: 6 }" /></div>
      <section v-else-if="loadError" class="load-error" role="alert"><h2>无法加载项目</h2><p>{{ loadError }}</p><a-button type="primary" @click="retryLoad">重试</a-button></section>
      <section v-else>
        <div class="section-head"><h2>最近项目</h2><router-link to="/video/projects">查看全部项目</router-link></div>
        <div v-if="projects.length === 0" class="empty-state"><h3>还没有视频项目</h3><p>新建项目后，从剧本开始逐步完成分镜、生成和导出。</p></div>
        <div v-else class="project-list">
          <button v-for="project in projects" :key="project.id" type="button" @click="continueProject(project)">
            <span class="project-name">{{ project.name }}</span>
            <span>{{ project.completedShots }}/{{ project.totalShots }} 个分镜完成</span>
            <a-progress :percent="project.totalShots ? Math.round(project.completedShots / project.totalShots * 100) : 0" :show-info="false" />
            <span class="continue-label">继续制作</span>
          </button>
        </div>
      </section>
    </main>
  </div>
</template>

<style scoped>
.production-page { min-height:100%; padding:24px; color:#1e293b; background:#f8fafc; }.production-header { max-width:1100px; margin:0 auto 28px; padding-bottom:20px; display:flex; align-items:center; justify-content:space-between; gap:16px; border-bottom:1px solid #cbd5e1; }.production-header h1 { margin:0 0 6px; font-size:26px; letter-spacing:0; }.production-header p { margin:0; color:#64748b; }main { max-width:1100px; margin:0 auto; }.section-head { display:flex; align-items:center; justify-content:space-between; }.project-list { display:grid; border-top:1px solid #dbe3ee; }.project-list button { min-height:76px; padding:12px 8px; display:grid; grid-template-columns:minmax(180px,1.4fr) minmax(150px,.8fr) minmax(160px,1fr) 90px; gap:16px; align-items:center; color:#475569; cursor:pointer; text-align:left; border:0; border-bottom:1px solid #dbe3ee; background:transparent; }.project-list button:hover { background:#fff; }.project-list button:focus-visible { outline:3px solid rgba(37,99,235,.28); }.project-name { color:#0f172a; font-weight:600; }.continue-label { color:#1d4ed8; }.empty-state,.load-error { min-height:320px; display:flex; flex-direction:column; align-items:flex-start; justify-content:center; }:deep(.ant-btn) { min-height:44px; border-radius:6px; }
@media(max-width:680px){.production-page{padding:16px}.production-header{align-items:stretch;flex-direction:column}.project-list button{grid-template-columns:1fr}.continue-label{min-height:44px;display:flex;align-items:center}}
</style>
