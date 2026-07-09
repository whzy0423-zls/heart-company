<script setup lang="ts">
import { useRouter } from 'vue-router';
import {
  Button as AButton,
  Card as ACard,
  Result as AResult,
  Space as ASpace,
  Step as AStep,
  Steps as ASteps,
} from 'ant-design-vue';

const router = useRouter();
</script>

<template>
  <div class="short-mode-page">
    <a-result
      title="短片制工作台"
      sub-title="功能规划中：后续用于单条短片快速制作，不走完整项目制流程。"
    >
      <template #extra>
        <a-space wrap>
          <a-button @click="router.push('/video/production')">
            返回制片工作台
          </a-button>
          <a-button type="primary" @click="router.push('/video/projects')">
            先使用项目制
          </a-button>
        </a-space>
      </template>
    </a-result>

    <a-card title="短片制规划" class="roadmap-card">
      <a-steps direction="vertical" :current="0" status="process">
        <a-step title="脚本输入" description="输入单条短片脚本或创意梗概。" />
        <a-step title="资产选择" description="从人物、物品、场景、风格资产中快速选择。" />
        <a-step title="快速分镜" description="自动拆分 3-8 个关键镜头。" />
        <a-step title="一键生成" description="生成视频并合成为可导出的短片。" />
      </a-steps>
    </a-card>
  </div>
</template>

<style scoped lang="less">
.short-mode-page {
  min-height: 100%;
  padding: 24px;
  background: #f8fafc;
}

.roadmap-card {
  max-width: 760px;
  margin: 0 auto;
  border-radius: 16px;
  box-shadow: 0 10px 26px rgba(15, 23, 42, 0.06);
}

@media (max-width: 768px) {
  .short-mode-page {
    padding: 16px;
  }
}
</style>
