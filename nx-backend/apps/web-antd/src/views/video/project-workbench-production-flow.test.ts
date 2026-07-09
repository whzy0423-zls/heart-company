import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../..');
const workbench = () =>
  readFileSync(resolve(root, 'src/views/video/projects/workbench.vue'), 'utf8');

const sectionBetween = (source: string, start: string, end: string) => {
  const startIndex = source.indexOf(start);
  const endIndex = source.indexOf(end, startIndex + start.length);

  expect(startIndex).toBeGreaterThanOrEqual(0);
  expect(endIndex).toBeGreaterThan(startIndex);

  return source.slice(startIndex, endIndex);
};

describe('project workbench production flow migration', () => {
  it('exposes the five migrated production steps', () => {
    const source = workbench();

    for (const label of ['剧本录入', '资产分析', '创建资产', '分镜设计', '剪辑合成']) {
      expect(source).toContain(label);
    }

    expect(source).toContain('productionSteps');
    expect(source).toContain('activeProductionStep');
  });

  it('bridges each step to current project-mode capabilities', () => {
    const source = workbench();

    expect(source).toContain('activeProductionStepMeta');
    expect(source).toContain('准备拆解资产和分镜');
    expect(source).toContain('角色 {{ characters.length }}');
    expect(source).toContain('场景 {{ scenes.length }}');
    expect(source).toContain('分镜 {{ shots.length }}');
    expect(source).toContain('showAddCharacter');
    expect(source).toContain('showAddScene');
    expect(source).toContain('showAddShot');
    expect(source).toContain('generateAllShots');
    expect(source).toContain('composeVideo');
  });

  it('makes the Aliyun bucket asset requirement visible in asset creation', () => {
    const source = workbench();

    expect(source).toContain('资产统一上传到阿里云 OSS 文件桶');
    expect(source).toContain('人物');
    expect(source).toContain('物品');
    expect(source).toContain('场景');
    expect(source).toContain('音频');
    expect(source).toContain('视频');
  });

  it('keeps the expanded workbench scrollable and responsive after adding the flow panel', () => {
    const source = workbench();
    const medium = sectionBetween(
      source,
      '@media (max-width: 1200px) and (min-width: 901px)',
      '@media (max-width: 900px)',
    );

    expect(source).toContain('compact-production-flow');
    expect(source).toContain('step-bar');
    expect(source).not.toContain('class="step-body"');
    expect(source).toContain('.workbench-layout {');
    expect(source).toContain('flex: 1 1 0;');
    expect(source).toContain('min-height: 0;');
    expect(source).toContain('@media (max-width: 1200px) and (min-width: 901px)');
    expect(source).toContain('grid-template-columns: minmax(220px, 280px) minmax(0, 1fr);');
    expect(source).toContain('grid-column: 1 / -1;');
    expect(medium).toContain('width: 100% !important;');
    expect(medium).toContain('max-width: 100%;');
    expect(medium).toContain('grid-template-columns: repeat(auto-fit, minmax(min(120px, 100%), 1fr));');
    expect(medium).toContain('overflow-wrap: anywhere;');
    expect(source).toContain('@media (max-width: 900px)');
    expect(source).toContain('grid-template-columns: repeat(2, minmax(0, 1fr));');
    expect(source).toContain('flex-direction: column;');
    expect(source).toContain('width: 100% !important;');
    expect(source).toContain('max-width: 100% !important;');
  });

  it('imports Ant Design components used by the a-prefixed production flow UI', () => {
    const source = workbench();

    for (const alias of [
      'Breadcrumb as ABreadcrumb',
      'Button as AButton',
      'Form as AForm',
      'Input as AInput',
      'Textarea as ATextarea',
    ]) {
      expect(source).toContain(alias);
    }
  });

  it('adds and edits shots inline instead of using the old modal form', () => {
    const source = workbench();
    const shotsList = sectionBetween(
      source,
      '<div class="shots-list">',
      '<!-- 右侧：分镜详情和参考素材 -->',
    );
    const showAddShot = sectionBetween(
      source,
      'function showAddShot() {',
      'async function handleAddShot()',
    );
    const handleAddShot = sectionBetween(
      source,
      'async function handleAddShot() {',
      'function editShot(shot: Shot) {',
    );
    const editShot = sectionBetween(
      source,
      'function editShot(shot: Shot) {',
      'function cancelShotInlineEdit() {',
    );

    expect(source).toContain('inlineShotFormVisible');
    expect(source).toContain('shot-edit-card');
    expect(source).toContain('shot-inline-form');
    expect(source).toContain('直接在分镜框里填写信息');
    expect(source).toContain('cancelShotInlineEdit');
    expect(source).toMatch(/<a-button[^>]*@click="showAddShot"[^>]*>\s*<PlusOutlined \/> 添加分镜\s*<\/a-button>/);
    expect(shotsList.indexOf('v-for="(shot, index) in shots"')).toBeLessThan(
      shotsList.indexOf('v-if="inlineShotFormVisible && !editingShotId"'),
    );
    expect(shotsList).toContain('v-if="inlineShotFormVisible && !editingShotId"');
    expect(shotsList).toContain('class="shot-card shot-edit-card"');
    expect(shotsList).toContain('@click.stop');
    expect(shotsList).toContain('<strong>新增分镜</strong>');
    expect(source).toContain(':disabled="shotMutationBusy" @click="showAddShot"');
    expect(shotsList).toContain(':loading="shotLoading" @click="handleAddShot"');
    expect(shotsList).toContain('v-if="shots.length === 0 && !inlineShotFormVisible"');
    expect(shotsList).toContain(':disabled="shotLoading" @click="cancelShotInlineEdit"');
    expect(shotsList).toContain('isShotActionBusy(shot.id)');
    expect(shotsList).toContain('@click="generateShot(shot)"');
    expect(shotsList).toContain('@click="editShot(shot)"');
    expect(shotsList).toContain('@click="deleteShot(shot.id)"');
    expect(showAddShot).toContain("editingShotId.value = ''");
    expect(showAddShot).toContain('if (shotMutationBusy.value) return');
    expect(showAddShot).toContain('inlineShotFormVisible.value = true');
    expect(showAddShot).not.toContain('Modal');
    expect(showAddShot).not.toContain('shotModal');
    expect(handleAddShot).toContain('if (shotLoading.value) return');
    expect(handleAddShot).toContain('shotToPayload(originalShot)');
    expect(handleAddShot).toContain('getNextShotOrderNum()');
    expect(handleAddShot).toContain('inlineShotFormVisible.value = false');
    expect(editShot).toContain('if (shotMutationBusy.value) return');
    expect(editShot).toContain('editingShotId.value = shot.id');
    expect(editShot).toContain('inlineShotFormVisible.value = true');
    expect(source).toContain('请先保存或取消当前分镜编辑');
    expect(source).toContain('selectedShot.value = null');
    expect(source).toContain('productionSecondaryDisabled');
    expect(source).toContain('shotMutationBusy');
    expect(source).toContain('if (shotMutationBusy.value) return');
    expect(source).not.toContain('shotModalVisible');
    expect(source).not.toContain('添加分镜对话框');
  });

  it('keeps production-flow actions consistent with existing generation and preview behavior', () => {
    const source = workbench();

    expect(source).toContain(':loading="generating"');
    expect(source).toContain(':disabled="shotMutationBusy"');
    expect(source).toContain('img,\n  video');
  });

  it('guards shot mutations while inline editing and scrolls the inline card into view', () => {
    const source = workbench();
    const generateShot = sectionBetween(
      source,
      'async function generateShot(shot: Shot) {',
      'async function generateAllShots()',
    );
    const generateAllShots = sectionBetween(
      source,
      'async function generateAllShots() {',
      '// 辅助函数',
    );
    const composeVideo = sectionBetween(
      source,
      'async function composeVideo() {',
      'function onDragStart',
    );
    const deleteShot = sectionBetween(
      source,
      'async function deleteShot(shotId: string) {',
      'function selectShot',
    );
    const dropToShot = sectionBetween(
      source,
      'async function onDropToShot(event: DragEvent, shot: Shot) {',
      '// 初始化',
    );

    expect(source).toContain('nextTick');
    expect(source).toContain('shotEditCardRef');
    expect(source).toContain('scrollShotInlineFormIntoView');
    expect(source).toContain('getNextShotOrderNum');
    expect(source).toContain('shotActionBusyIds');
    expect(source).toContain('isShotActionBusy');
    expect(source).toContain('bindingShotIds');
    expect(source).toContain('请先保存或取消当前分镜编辑');

    expect(generateShot).toContain('hasOpenShotInlineForm.value');
    expect(generateShot).toContain('shotActionBusyIds.value');
    expect(generateShot).toContain('shotActionBusyIds.value.add(shot.id)');
    expect(generateShot).toContain('shotActionBusyIds.value.delete(shot.id)');
    expect(generateAllShots).toContain('hasOpenShotInlineForm.value');
    expect(generateAllShots).toContain('message.warning');
    expect(composeVideo).toContain('hasOpenShotInlineForm.value');
    expect(composeVideo).toContain('message.warning');
    expect(deleteShot).toContain('shotActionBusyIds.value.add(shotId)');
    expect(deleteShot).toContain('shotActionBusyIds.value.delete(shotId)');
    expect(dropToShot).toContain('shotMutationBusy.value || hasOpenShotInlineForm.value');
    expect(dropToShot).toContain('bindingShotIds.value.has(shot.id)');
    expect(dropToShot).toContain('bindingShotIds.value.add(shot.id)');
    expect(dropToShot).toContain('bindingShotIds.value.delete(shot.id)');
    expect(dropToShot).toContain('const latestShot = shots.value.find');
  });
});
