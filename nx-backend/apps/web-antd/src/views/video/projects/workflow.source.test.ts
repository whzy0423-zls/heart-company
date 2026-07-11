import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

const root = path.resolve(import.meta.dirname);
const read = (file: string) => fs.readFileSync(path.join(root, file), 'utf8');

describe('guided workflow source contracts', () => {
  it('mounts the guided workflow by default and preserves hidden advanced route', () => {
    const routes = read('../../../router/routes/modules/video.ts');
    expect(routes).toContain("import('#/views/video/projects/workflow.vue')");
    expect(routes).toContain("path: 'projects/:id/workbench/advanced'");
    expect(routes).toContain("import('#/views/video/projects/workbench.vue')");
    expect(routes).not.toContain("path: 'production/short'");
  });

  it('renders five real steps and one primary action shell', () => {
    const shell = read('workflow.vue');
    const stepper = read('workflow/WorkflowStepper.vue');
    for (const label of ['项目与剧本', '角色与场景', '分镜', '生成', '导出']) {
      expect(stepper).toContain(label);
    }
    for (const step of ['brief', 'assets', 'storyboard', 'generate', 'export']) {
      expect(stepper).toContain(`'${step}'`);
    }
    expect(stepper).toContain('aria-current');
    expect(shell).toContain('aria-live="polite"');
    expect(shell).toContain('data-primary-action');
    expect(shell.match(/data-primary-action/g)).toHaveLength(1);
    expect(shell).toContain("router.replace({ query: { ...route.query, step } })");
    expect(shell).toContain('/workbench/advanced');
    expect(shell).not.toContain('workbenchMode');
  });

  it('provides loading, fatal retry, safe footer and reduced motion', () => {
    const shell = read('workflow.vue');
    expect(shell).toContain('workflow-loading');
    expect(shell).toContain('workflow-fatal');
    expect(shell).toContain('retryLoad');
    expect(shell).toContain('min-height: 44px');
    expect(shell).toContain('env(safe-area-inset-bottom)');
    expect(shell).toContain('@media (prefers-reduced-motion: reduce)');
  });

  it('provides complete brief, assets and storyboard step controls', () => {
    const brief = read('workflow/BriefStep.vue');
    const assets = read('workflow/AssetsStep.vue');
    const storyboard = read('workflow/StoryboardStep.vue');

    for (const text of ['项目名称', '主题', '完整剧本', '视觉风格', 'scriptCharacterCount', 'estimatedParagraphCount', 'role="alert"']) {
      expect(brief).toContain(text);
    }
    for (const text of ['角色', '场景', '缺少参考图', '添加角色', '添加场景', '打开资产库', '重试']) {
      expect(assets).toContain(text);
    }
    for (const text of ['从剧本创建分镜', '手动添加', 'created', 'existing', 'failed', 'retryFailed', 'mobile-shot-selector', '动作描述', '角色', '场景', '时长', '画幅']) {
      expect(storyboard).toContain(text);
    }
    expect(`${brief}${assets}${storyboard}`).toContain('min-height: 44px');
  });

  it('provides safe generation filters, recovery and explicit version selection', () => {
    const generation = read('workflow/GenerationStep.vue');
    const drawer = read('workflow/VersionDrawer.vue');
    const polling = read('workflow/useWorkflowPolling.ts');
    for (const label of ['可生成', '待完善', '生成中', '已完成']) expect(generation).toContain(label);
    for (const text of ['ready', 'stale', 'failed', 'recovery', 'requestKeys', 'crypto.randomUUID', 'batchGenerateShotsSafeApi', 'generateShotSafeApi', '检查结果', '对账']) expect(generation).toContain(text);
    expect(generation).toContain('再生成一个版本');
    expect(drawer).toContain('设为当前');
    expect(drawer).toContain('setShotVideoVersionApi');
    expect(drawer).not.toContain('autoSelect');
    expect(polling).toContain('clearTimeout');
    expect(polling).toContain('maxAttempts');
  });

  it('provides exact export participation, async jobs and stale result recovery', () => {
    const exportStep = read('workflow/ExportStep.vue');
    for (const text of ['includedShotIds', 'excludedShotIds', 'partialAcknowledged', 'composeProjectSafeApi', 'getComposeJobApi', 'jobId', 'progress', '重试合成', '内容已变化，需要重新合成', '复制链接', '下载成片']) expect(exportStep).toContain(text);
    expect(exportStep).toContain('clearTimeout');
  });
});
