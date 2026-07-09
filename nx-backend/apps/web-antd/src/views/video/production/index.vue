<script setup lang="ts">
import { useRouter } from 'vue-router';
import { Button as AButton, Card as ACard, Tag as ATag } from 'ant-design-vue';

const router = useRouter();

function goProjectMode() {
  router.push('/video/projects');
}

function goShortMode() {
  router.push('/video/production/short');
}
</script>

<template>
  <div class="production-page">
    <section class="production-hero">
      <a-tag color="blue">Production Workspace</a-tag>
      <h1>制片工作台</h1>
      <p>
        将剧本、资产、分镜、生成与剪辑合成串成完整生产流。当前先支持项目制完整链路，短片制保留独立入口。
      </p>
    </section>

    <div class="mode-grid">
      <a-card class="mode-card mode-card-primary" hoverable @click="goProjectMode">
        <template #title>项目制</template>
        <template #extra>
          <a-tag color="green">已接入</a-tag>
        </template>
        <p>适合连续剧、系列视频、动画项目等完整制作流程。</p>
        <ul>
          <li>项目管理与项目配置</li>
          <li>资产统一上传到阿里云 OSS 文件桶</li>
          <li>分镜设计、批量生成与成片合成</li>
        </ul>
        <a-button type="primary" block @click.stop="goProjectMode">
          进入项目制
        </a-button>
      </a-card>

      <a-card class="mode-card" hoverable @click="goShortMode">
        <template #title>短片制</template>
        <template #extra>
          <a-tag>占位</a-tag>
        </template>
        <p>面向单条短片快速制作，后续会提供更轻量的一站式流程。</p>
        <ul>
          <li>脚本输入</li>
          <li>快速分镜</li>
          <li>一键生成与导出</li>
        </ul>
        <a-button block @click.stop="goShortMode">查看短片制规划</a-button>
      </a-card>
    </div>
  </div>
</template>

<style scoped lang="less">
.production-page {
  min-height: 100%;
  padding: 24px;
  background:
    radial-gradient(circle at top left, rgba(37, 99, 235, 0.12), transparent 32%),
    #f8fafc;
}

.production-hero {
  max-width: 880px;
  margin-bottom: 24px;
  padding: 28px;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 18px;
  box-shadow: 0 12px 30px rgba(15, 23, 42, 0.06);

  h1 {
    margin: 12px 0 8px;
    color: #0f172a;
    font-size: 32px;
    font-weight: 700;
    line-height: 1.2;
  }

  p {
    max-width: 720px;
    margin: 0;
    color: #475569;
    font-size: 16px;
    line-height: 1.7;
  }
}

.mode-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 20px;
  max-width: 1120px;
}

.mode-card {
  background: #fff;
  color: #0f172a;
  border-color: #e2e8f0;
  border-radius: 16px;
  transition:
    transform 0.2s ease,
    box-shadow 0.2s ease,
    border-color 0.2s ease;

  :deep(.ant-card-head) {
    min-height: 56px;
    background: #fff;
  }

  :deep(.ant-card-body) {
    background: #fff;
  }

  :deep(.ant-btn-default) {
    background: #fff;
    color: #0f172a;
    border-color: #cbd5e1;
  }

  :deep(.ant-card-head-title) {
    color: #0f172a;
    font-size: 18px;
    font-weight: 650;
  }

  p {
    min-height: 48px;
    margin-bottom: 16px;
    color: #475569;
    line-height: 1.6;
  }

  ul {
    min-height: 92px;
    margin: 0 0 20px;
    padding-left: 18px;
    color: #334155;
    line-height: 1.9;
  }

  &:hover {
    transform: translateY(-2px);
    box-shadow: 0 16px 36px rgba(15, 23, 42, 0.1);
  }
}

.mode-card-primary {
  border-color: rgba(37, 99, 235, 0.28);
}

@media (max-width: 768px) {
  .production-page {
    padding: 16px;
  }

  .production-hero {
    padding: 20px;

    h1 {
      font-size: 26px;
    }
  }

  .mode-grid {
    grid-template-columns: 1fr;
  }
}
</style>
