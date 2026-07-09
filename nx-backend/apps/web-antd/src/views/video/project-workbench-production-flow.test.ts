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

  it('exposes the full OSS asset library entry from the asset creation step', () => {
    const source = workbench();
    const primaryLabels = sectionBetween(
      source,
      'const productionPrimaryActionLabel = computed(() => {',
      'const productionSecondaryActionLabel = computed(() => {',
    );
    const primaryAction = sectionBetween(
      source,
      'function handleProductionPrimaryAction() {',
      'function handleProductionSecondaryAction() {',
    );

    expect(source).toContain('useRouter');
    expect(source).toContain('const router = useRouter()');
    expect(source).toContain('openVideoAssetLibrary');
    expect(source).toContain("router.push('/video/assets')");
    expect(primaryLabels).toContain("assets: '打开资产库'");
    expect(primaryAction).toContain("case 'assets'");
    expect(primaryAction).toContain('openVideoAssetLibrary()');
    expect(source).toContain('人物/场景/物品/服装/音频/视频');
    expect(source).toContain('统一上传到 OSS 后可在分镜参考素材中选择使用');
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

  it('prevents the storyboard columns from creating horizontal scroll inside the desktop workbench', () => {
    const source = workbench();
    const workbenchContent = sectionBetween(source, '.workbench-content {', '\n}');
    const boardBody = sectionBetween(source, '.board-body {', '\n}');
    const boardCol = sectionBetween(source, '.board-col {', '\n}');

    expect(workbenchContent).toContain('overflow-x: hidden;');
    expect(boardBody).toContain(
      'grid-template-columns: minmax(220px, 1.05fr) minmax(220px, 0.9fr) minmax(180px, 0.72fr);',
    );
    expect(boardCol).toContain('overflow-wrap: anywhere;');
  });

  it('relays wheel events from the side panels to the scrollable shot list so the page never feels locked', () => {
    const source = workbench();
    const layoutTemplate = sectionBetween(
      source,
      'class="workbench-layout"',
      '<div class="shots-header">',
    );
    const workbenchLayout = sectionBetween(source, '.workbench-layout {', '\n}');
    const workbenchContent = sectionBetween(source, '.workbench-content {', '\n}');
    const medium = sectionBetween(
      source,
      '@media (max-width: 1200px) and (min-width: 901px)',
      '@media (max-width: 900px)',
    );

    expect(layoutTemplate).toContain('@wheel.passive="handleWorkbenchWheel"');
    expect(layoutTemplate).toContain('ref="workbenchContentRef"');
    expect(source).toContain('workbenchContentRef');
    expect(source).toContain('getWorkbenchContentElement');
    expect(source).toContain('findScrollableWorkbenchWheelTarget');
    expect(source).toContain('handleWorkbenchWheel');
    expect(source).toContain('workbenchContent.scrollTop += event.deltaY');
    expect(workbenchLayout).toContain('overflow: hidden;');
    expect(workbenchContent).toContain('overflow-y: auto;');
    expect(medium).toContain('overflow: hidden;');
  });

  it('keeps the main shot list full-width in the narrow mobile workbench layout', () => {
    const source = workbench();
    const narrow = sectionBetween(source, '@media (max-width: 900px)', '</style>');
    const narrowWorkbenchLayout = sectionBetween(narrow, '  .workbench-layout {', '\n  }');
    const narrowWorkbenchContent = sectionBetween(narrow, '  .workbench-content {', '\n  }');

    expect(narrowWorkbenchLayout).toContain('overflow-x: hidden;');
    expect(narrowWorkbenchLayout).toContain('overflow-y: auto;');
    expect(narrowWorkbenchContent).toContain('width: 100% !important;');
    expect(narrowWorkbenchContent).toContain('max-width: 100% !important;');
    expect(narrowWorkbenchContent).toContain('flex: none !important;');
    expect(narrowWorkbenchContent).toContain('overflow: visible;');
  });

  it('keeps the video generation detail modal scrollable and single-column on small screens', () => {
    const source = workbench();
    const narrow = sectionBetween(source, '@media (max-width: 900px)', '</style>');
    const detailGrid = sectionBetween(narrow, '  .video-generation-detail-grid {', '\n  }');
    const detailMeta = sectionBetween(narrow, '  .detail-meta {', '\n  }');

    expect(source).toContain('wrap-class-name="video-generation-detail-modal"');
    expect(source).toContain(':global(.video-generation-detail-modal .ant-modal-content)');
    expect(source).toContain('max-height: calc(100vh - 48px);');
    expect(source).toContain(':global(.video-generation-detail-modal .ant-modal-body)');
    expect(source).toContain('overflow-y: auto;');
    expect(detailGrid).toContain('grid-template-columns: 1fr;');
    expect(detailMeta).toContain('grid-template-columns: repeat(2, minmax(0, 1fr));');
  });

  it('keeps the production flow title readable in the desktop header', () => {
    const source = workbench();
    const flowPanel = sectionBetween(source, '.production-flow-panel {', '\n}');
    const flowTitleHeading = sectionBetween(source, '  h2 {', '\n  }');

    expect(flowPanel).toContain(
      'grid-template-columns: minmax(360px, 420px) minmax(480px, 1fr) minmax(280px, auto);',
    );
    expect(flowTitleHeading).toContain('white-space: nowrap;');
    expect(flowTitleHeading).toContain('text-overflow: ellipsis;');
  });

  it('keeps four-character production step labels visible in compact desktop pills', () => {
    const source = workbench();
    const stepItem = sectionBetween(source, '.step-item {', '\n}');

    expect(stepItem).toContain('padding: 6px 8px;');
    expect(stepItem).toContain('gap: 5px;');
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

  it('uploads character and scene references to the OSS bucket and previews them instead of asking for urls', () => {
    const source = workbench();
    const characterModal = sectionBetween(
      source,
      '<!-- 添加角色对话框 -->',
      '<!-- 添加场景对话框 -->',
    );
    const sceneModal = sectionBetween(
      source,
      '<!-- 添加场景对话框 -->',
      '<!-- 预览提示词对话框 -->',
    );

    expect(source).toContain('uploadFileApi');
    expect(source).toContain('useUploadAssetPreviewUrl');
    expect(source).toContain('useUploadAssetPreviewResolver');
    expect(source).toContain('uploadWorkbenchReference');
    expect(source).toContain('assetRefPreview');
    expect(source).toContain('characterImagePreviewUrl');
    expect(source).toContain('sceneImagePreviewUrl');
    expect(source).toContain('sceneVideoPreviewUrl');

    expect(characterModal).toContain('上传角色参考图');
    expect(characterModal).toContain('accept="image/*"');
    expect(characterModal).toContain('@click="clearCharacterReferenceImage"');
    expect(characterModal).toContain('characterImagePreviewUrl');
    expect(characterModal).not.toContain('placeholder="图片 URL"');

    expect(sceneModal).toContain('上传场景参考图');
    expect(sceneModal).toContain('上传场景参考视频');
    expect(sceneModal).toContain('accept="image/*"');
    expect(sceneModal).toContain('accept="video/*"');
    expect(sceneModal).toContain('@click="clearSceneReferenceImage"');
    expect(sceneModal).toContain('@click="clearSceneReferenceVideo"');
    expect(sceneModal).toContain('sceneImagePreviewUrl');
    expect(sceneModal).toContain('sceneVideoPreviewUrl');
    expect(sceneModal).not.toContain('placeholder="图片 URL"');
    expect(sceneModal).not.toContain('placeholder="视频 URL"');
  });

  it('moves project shots toward liuguang board cards with script, generate, and version columns', () => {
    const source = workbench();

    for (const className of [
      'board-list',
      'board-card',
      'board-head',
      'board-body',
      'col-left',
      'col-generate',
      'col-version',
    ]) {
      expect(source).toContain(className);
    }

    expect(source).toContain('分镜剧本');
    expect(source).toContain('scriptOriginalContent');
    expect(source).toContain('handleScriptOriginalContentChange');
    expect(source).toContain('视频生成');
    expect(source).toContain('视频版本');
  });

  it('supports shot-level image video and audio references through OSS upload with preview', () => {
    const source = workbench();

    for (const token of [
      'shotAssets',
      'uploadShotReferenceAsset',
      'uploadShotReferenceImage',
      'uploadShotReferenceVideo',
      'uploadShotReferenceAudio',
      'createShotAssetApi',
      'deleteShotAssetApi',
      'getShotAssetsByType',
      'video/shot-image',
      'video/shot-video',
      'video/shot-audio',
      'accept="image/*"',
      'accept="video/*"',
      'accept="audio/*"',
    ]) {
      expect(source).toContain(token);
    }

    expect(source).toContain('上传图片');
    expect(source).toContain('上传视频');
    expect(source).toContain('上传音频');
    expect(source).toContain('controls');
    expect(source).toContain('previewReferenceAsset(asset.objectUrl)');
  });

  it('stores only public OSS objectUrl values for generation reference assets', () => {
    const source = workbench();
    const publicUrlHelpers = sectionBetween(
      source,
      'function isPublicHttpUrl(url?: string) {',
      'async function uploadWorkbenchReference',
    );
    const uploadWorkbenchReference = sectionBetween(
      source,
      'async function uploadWorkbenchReference(',
      'function uploadCharacterReferenceImage',
    );
    const pickAsset = sectionBetween(
      source,
      'async function handlePickShotAsset(asset: VideoAsset) {',
      'async function uploadShotReferenceAsset',
    );
    const uploadShotReferenceAsset = sectionBetween(
      source,
      'async function uploadShotReferenceAsset(',
      'async function syncShotReferenceAssetChange',
    );

    expect(publicUrlHelpers).toContain('requireUploadedPublicObjectUrl');
    expect(publicUrlHelpers).toContain('requirePublicReferenceUrl');
    expect(publicUrlHelpers).toContain("value.startsWith('http://') || value.startsWith('https://')");
    expect(uploadWorkbenchReference).toContain('assign(requireUploadedPublicObjectUrl(result, label))');
    expect(uploadWorkbenchReference).not.toContain('assign(result.objectUrl || result.url)');
    expect(pickAsset).toContain("requirePublicReferenceUrl(getAssetPreviewSource(asset), '资产库素材')");
    expect(uploadShotReferenceAsset).toContain('objectUrl: requireUploadedPublicObjectUrl(result, label)');
    expect(uploadShotReferenceAsset).not.toContain('objectUrl: result.objectUrl || result.url');
  });

  it('keeps selected shot and open generation detail in sync after shot reference asset changes', () => {
    const source = workbench();
    const syncHelper = sectionBetween(
      source,
      'async function syncShotReferenceAssetChange(shotId: string) {',
      'function uploadShotReferenceImage',
    );
    const pickAsset = sectionBetween(
      source,
      'async function handlePickShotAsset(asset: VideoAsset) {',
      'async function uploadShotReferenceAsset',
    );
    const uploadAsset = sectionBetween(
      source,
      'async function uploadShotReferenceAsset(',
      'function uploadShotReferenceImage',
    );
    const deleteAsset = sectionBetween(
      source,
      'function deleteShotAsset(asset: ShotAsset) {',
      'async function handleVideoGenerationParamsChange',
    );
    const extractFrame = sectionBetween(
      source,
      'async function handleExtractShotVideoFrame(shot: Shot, version: ShotVideoVersion) {',
      'function handleOpenCopyShotVideoVersion',
    );

    expect(syncHelper).toContain('selectedShot.value = latestShot');
    expect(syncHelper).toContain('videoGenerationDetailShot.value = latestShot');
    expect(syncHelper).toContain('await reloadOpenVideoGenerationDetail(latestShot)');
    expect(pickAsset).toContain('await syncShotReferenceAssetChange(targetShot.id)');
    expect(uploadAsset).toContain('await syncShotReferenceAssetChange(shot.id)');
    expect(deleteAsset).toContain('await syncShotReferenceAssetChange(asset.shotId)');
    expect(extractFrame).toContain('await syncShotReferenceAssetChange(shot.id)');
  });

  it('auto-selects the first loaded shot so the right detail preview is not empty on entry', () => {
    const source = workbench();
    const loadShots = sectionBetween(
      source,
      'async function loadShots() {',
      '// 角色管理',
    );

    expect(loadShots).toContain('const previousSelectedShotId = selectedShot.value?.id');
    expect(loadShots).toContain('if (previousSelectedShotId)');
    expect(loadShots).toContain('shots.value.find((shot) => shot.id === previousSelectedShotId)');
    expect(loadShots).toContain('selectedShot.value = shots.value[0] || null');
  });

  it('keeps used audio references in shot detail displays like images and videos', () => {
    const source = workbench();
    const collectRefs = sectionBetween(
      source,
      'function collectShotReferenceAssets(shot: Shot | null): VideoGenerationDetailReference[] {',
      'function getShotReferenceDisplayAssets',
    );

    expect(source).toContain('usedAudios');
    expect(collectRefs).toContain('for (const audio of shot.usedAudios || [])');
    expect(collectRefs).toContain("pushReference('audio', audio, '生成使用音频')");
  });

  it('keeps video generation detail references scoped to the loaded version even when empty', () => {
    const source = workbench();
    const detailRefs = sectionBetween(
      source,
      'function getVideoGenerationDetailReferences(shot: Shot | null): VideoGenerationDetailReference[] {',
      'function getShotVideoVersions',
    );
    const openDetail = sectionBetween(
      source,
      'async function handleOpenVideoGenerationDetail(shot: Shot, version: ShotVideoVersion) {',
      'function handleCloseVideoGenerationDetail',
    );
    const applyDetail = sectionBetween(
      source,
      'function applyVideoGenerationDetail(',
      'async function reloadOpenVideoGenerationDetail',
    );

    expect(source).toContain('videoGenerationDetailReferencesLoaded');
    expect(detailRefs).toContain('videoGenerationDetailReferencesLoaded.value');
    expect(detailRefs).toContain('return videoGenerationDetailReferences.value');
    expect(openDetail).toContain('videoGenerationDetailReferencesLoaded.value = false');
    expect(openDetail).toContain('applyVideoGenerationDetail(detail, shot, version)');
    expect(applyDetail).toContain('videoGenerationDetailReferencesLoaded.value = true');
  });

  it('uses canonical shotAssets in the right detail panel so uploaded references stay visible after refresh', () => {
    const source = workbench();
    const rightDetailPanel = sectionBetween(
      source,
      '<!-- 右侧：分镜详情和参考素材 -->',
      '<a-tab-pane key="prompt" tab="提示词">',
    );

    expect(source).toContain('getShotReferenceDisplayAssets');
    expect(rightDetailPanel).toContain('getShotReferenceDisplayAssets(selectedShot)');
    expect(rightDetailPanel).toContain("reference.type === 'image'");
    expect(rightDetailPanel).toContain("reference.type === 'video'");
    expect(rightDetailPanel).toContain("reference.type === 'audio'");
    expect(rightDetailPanel).toContain('reference-audio-item');
    expect(rightDetailPanel).toContain('previewReferenceAsset(reference.url)');
    expect(rightDetailPanel).toContain('getShotReferenceDisplayAssets(selectedShot).length === 0');
    expect(rightDetailPanel).not.toContain('v-for="img in selectedShot.usedImages"');
    expect(rightDetailPanel).not.toContain('v-for="video in selectedShot.usedVideos"');
  });

  it('can attach existing OSS asset-library items to a shot reference like liuguang', () => {
    const source = workbench();
    const shotReferenceSection = sectionBetween(
      source,
      '<span class="col-title">参考素材（{{ getShotReferenceAssetCount(shot) }}）</span>',
      '<div class="shot-reference-grid">',
    );

    for (const token of [
      'AssetPicker',
      'VideoAsset, VideoAssetType',
      'getAssetPreviewSource',
      'shotAssetPickerOpen',
      'shotAssetPickerAllowTypes',
      'openShotAssetPicker',
      'handlePickShotAsset',
      "['scene', 'character', 'prop', 'outfit', 'style']",
      "['video']",
      "['audio']",
      'createShotAssetApi(targetShot.id',
      '资产库图片',
      '资产库视频',
      '资产库音频',
      '已从资产库添加到分镜参考素材',
    ]) {
      expect(source).toContain(token);
    }

    expect(shotReferenceSection).toContain("@click.stop=\"openShotAssetPicker(shot, 'image')\"");
    expect(shotReferenceSection).toContain("@click.stop=\"openShotAssetPicker(shot, 'video')\"");
    expect(shotReferenceSection).toContain("@click.stop=\"openShotAssetPicker(shot, 'audio')\"");
    expect(source).toContain(':allow-types="shotAssetPickerAllowTypes"');
    expect(source).toContain('@pick="handlePickShotAsset"');
  });

  it('adds the liuguang-style video generation parameter panel for each shot', () => {
    const source = workbench();

    for (const token of [
      'videoModelOptions',
      'soundAndPictureTogetherOptions',
      'handleVideoGenerationParamsChange',
      'selectedVideoModel',
      'soundAndPictureTogether',
      'dynamicDescription',
      'videoEstimateMap',
      '积分预估',
      '选择生视频模型',
      '启用/禁用音画同出',
      '动态描述',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('shows real video versions and version operations instead of placeholder only', () => {
    const source = workbench();

    for (const token of [
      'shotVideoVersions',
      'listShotVideoVersionsApi',
      'setShotVideoVersionApi',
      'deleteShotVideoVersionApi',
      'handleSetShotVideoVersion',
      'handleDeleteShotVideoVersion',
      '设为当前',
      '删除版本',
      '当前版本',
    ]) {
      expect(source).toContain(token);
    }

    expect(source).not.toContain('已预留 liuguang 视频版本能力');
  });

  it('can refresh shot video versions so async generated videos are visible', () => {
    const source = workbench();

    for (const token of [
      'refreshShotVideoVersionsApi',
      'handleRefreshShotVideoVersions',
      'shotVideoVersionRefreshIds',
      'isShotVideoVersionRefreshing',
      '刷新版本',
      '刷新生成状态',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('validates liuguang-style video generation params before submitting jobs', () => {
    const source = workbench();

    for (const token of [
      'getShotVideoPrompt',
      'validateShotVideoGenerationParams',
      'getGeneratableShots',
      '请输入视频描述',
      '没有符合条件的分镜（需要有视频描述、模型、分辨率和时长）',
      '符合条件的分镜正在生成视频，请稍候',
      'getGeneratableShots().length',
      'const validShots = getGeneratableShots()',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('polls shot generation status after generate so videos become visible automatically', () => {
    const source = workbench();

    for (const token of [
      'shotGenerationPollingIds',
      'shotGenerationPollingTimers',
      'startShotGenerationPolling',
      'stopShotGenerationPolling',
      'stopAllShotGenerationPolling',
      'isShotGenerationPolling',
      'handleRefreshShotVideoVersions(shot, { force: true, silent: true })',
      'startShotGenerationPolling(shot.id)',
      'window.setTimeout',
      'onBeforeUnmount',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('can copy a generated video version to another shot like liuguang', () => {
    const source = workbench();

    for (const token of [
      'copyShotVideoVersionApi',
      'copyShotVideoVersionModalVisible',
      'handleOpenCopyShotVideoVersion',
      'handleCopyShotVideoVersion',
      'copyShotVideoVersionTargetOptions',
      '复制到分镜',
      '选择目标分镜',
      '确认复制',
    ]) {
      expect(source).toContain(token);
    }
  });







  it('marks newly generated video versions as unviewed until the user views or closes the badge', () => {
    const source = workbench();

    for (const token of [
      'viewedFlag',
      'markShotVideoVersionViewedApi',
      'isUnviewedShotVideoVersion',
      'handleMarkShotVideoVersionViewed',
      'unviewed-badge',
      '未查看',
      'handleOpenShotVideoPreview(shot, version)',
      '@click.stop="handleMarkShotVideoVersionViewed(shot, version)"',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('copies a video version to an existing shot or a newly created shot like liuguang', () => {
    const source = workbench();

    for (const token of [
      'copyShotVideoVersionMode',
      'assign',
      'append',
      '指定分镜',
      '新增分镜',
      'handleCopyShotVideoVersionToNewShot',
      'createShotApi(projectId.value',
      'copyShotVideoVersionApi(sourceShot.id, sourceVersion.id, createdShot.id)',
      '已新增分镜并复制视频版本',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('opens a liuguang-style video generation detail dialog from each version', () => {
    const source = workbench();

    for (const token of [
      'getShotVideoVersionDetailApi',
      'videoGenerationDetailLoading',
      'videoGenerationDetailModalVisible',
      'videoGenerationDetailShot',
      'videoGenerationDetailVersion',
      'handleOpenVideoGenerationDetail',
      'handleCloseVideoGenerationDetail',
      'getVideoGenerationDetailReferences',
      'await getShotVideoVersionDetailApi(shot.id, version.id)',
      ':spinning="videoGenerationDetailLoading"',
      '视频生成详情加载失败',
      '视频生成详情',
      '生成参考内容',
      '视频结果',
      '查看详情',
      'detail-action',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('refreshes local video version lists after setting or deleting a version so current/deleted state is not stale', () => {
    const source = workbench();
    const setVersion = sectionBetween(
      source,
      'async function handleSetShotVideoVersion(shot: Shot, version: ShotVideoVersion) {',
      'async function handleSetShotVideoVersionBackup',
    );
    const deleteVersion = sectionBetween(
      source,
      'function handleDeleteShotVideoVersion(shot: Shot, version: ShotVideoVersion) {',
      'function estimateShotVideoPoints',
    );

    expect(setVersion).toContain('await loadShotVideoVersions([latestShot || shot])');
    expect(setVersion).toContain('selectedShot.value = latestShot');
    expect(setVersion).toContain('videoGenerationDetailShot.value = latestShot');
    expect(deleteVersion).toContain('await loadShotVideoVersions([latestShot || shot])');
    expect(deleteVersion).toContain('handleCloseVideoGenerationDetail()');
  });

  it('keeps other shot video version lists when refreshing only one shot', () => {
    const source = workbench();
    const loadVersions = sectionBetween(
      source,
      'async function loadShotVideoVersions(',
      'async function handleRefreshShotVideoVersions',
    );
    const loadShots = sectionBetween(
      source,
      'async function loadShots() {',
      '// 角色管理',
    );

    expect(loadVersions).toContain('options: { pruneStale?: boolean } = {}');
    expect(loadVersions).toContain('if (options.pruneStale)');
    expect(loadVersions).toContain('if (shotIds.length === 0) {');
    expect(loadShots).toContain('await loadShotVideoVersions(shots.value, { pruneStale: true })');
  });

  it('syncs an open video generation detail dialog after refreshing shot versions', () => {
    const source = workbench();
    const refreshVersions = sectionBetween(
      source,
      'async function handleRefreshShotVideoVersions(',
      'async function handleOpenVideoGenerationDetail',
    );

    expect(refreshVersions).toContain('const latestShot = shots.value.find((item) => item.id === shot.id) || shot');
    expect(refreshVersions).toContain('videoGenerationDetailShot.value = latestShot');
    expect(refreshVersions).toContain('const latestDetailVersion = (shotVideoVersions.value[shot.id] || []).find');
    expect(refreshVersions).toContain('videoGenerationDetailVersion.value = latestDetailVersion');
  });

  it('refreshes video versions immediately after submitting a shot generation or regeneration task', () => {
    const source = workbench();
    const regenerate = sectionBetween(
      source,
      'async function handleRegenerateShotVideoVersion(shot: Shot, version: ShotVideoVersion) {',
      'async function handleReeditShotVideoVersion',
    );
    const generate = sectionBetween(
      source,
      'async function generateShot(shot: Shot) {',
      'async function generateAllShots',
    );

    expect(regenerate).toContain('await loadShotVideoVersions([latestShot || shot])');
    expect(regenerate).toContain('startShotGenerationPolling(shot.id)');
    expect(generate).toContain('await loadShotVideoVersions([latestShot || shot])');
    expect(generate).toContain('startShotGenerationPolling(shot.id)');
  });

  it('refreshes submitted shot version lists immediately after batch generation', () => {
    const source = workbench();
    const batchGenerate = sectionBetween(
      source,
      'async function generateAllShots() {',
      'onMounted(() => {',
    );

    expect(batchGenerate).toContain('const submittedShots = shots.value.filter((shot) => submittedShotIds.includes(shot.id))');
    expect(batchGenerate).toContain('await loadShotVideoVersions(submittedShots)');
    expect(batchGenerate).toContain('pollingShotIds.forEach((shotId) => startShotGenerationPolling(shotId))');
  });

  it('refreshes target shot video versions immediately after copying a version', () => {
    const source = workbench();
    const copyExisting = sectionBetween(
      source,
      'async function handleCopyShotVideoVersion() {',
      'async function handleCopyShotVideoVersionToNewShot',
    );
    const copyNew = sectionBetween(
      source,
      'async function handleCopyShotVideoVersionToNewShot() {',
      'async function handleUseShotVideoVersion',
    );

    expect(copyExisting).toContain('const latestTargetShot = shots.value.find((item) => item.id === targetShotId)');
    expect(copyExisting).toContain('await loadShotVideoVersions([latestTargetShot])');
    expect(copyNew).toContain('const latestCreatedShot = shots.value.find((item) => item.id === createdShot.id)');
    expect(copyNew).toContain('await loadShotVideoVersions([latestCreatedShot || createdShot])');
  });

  it('exposes liuguang-style use regenerate and reedit actions on video versions', () => {
    const source = workbench();

    for (const token of [
      'handleUseShotVideoVersion',
      'handleRegenerateShotVideoVersion',
      'handleReeditShotVideoVersion',
      '使用此视频',
      '重新生成',
      '重编辑',
      'version.prompt',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('can extract a frame from a generated video version back into shot reference assets', () => {
    const source = workbench();

    for (const token of [
      'extractShotVideoFrameApi',
      'shotVideoFrameExtractingIds',
      'isShotVideoFrameExtracting',
      'handleExtractShotVideoFrame',
      '视频抽帧',
      '抽帧',
      'video/frames',
      'await loadShots()',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('opens generated video versions in a liuguang-style preview dialog', () => {
    const source = workbench();

    for (const token of [
      'shotVideoPreviewModalVisible',
      'shotVideoPreviewUrl',
      'shotVideoPreviewTitle',
      'handleOpenShotVideoPreview',
      'handleCloseShotVideoPreview',
      '视频预览',
      '预览视频',
      'detail-video-preview',
      'preview-dialog-video',
      '@click.stop="handleOpenShotVideoPreview(shot, version)"',
      '@click="handleOpenShotVideoPreview(videoGenerationDetailShot, videoGenerationDetailVersion)"',
      'handleMarkShotVideoVersionViewed(shot, version)',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('can mark a video version as liuguang-style backup video', () => {
    const source = workbench();

    for (const token of [
      'backupFlag',
      'setShotVideoVersionBackupApi',
      'handleSetShotVideoVersionBackup',
      'isShotVideoVersionBackup',
      '备选',
      '设为备选',
      '取消备选',
      'backup-video-tag',
      '@click.stop="handleSetShotVideoVersionBackup(shot, version)"',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('uses liuguang-style video thumbnails with a play overlay in version cards', () => {
    const source = workbench();

    for (const token of [
      'shotVideoThumbnailFailedUrls',
      'supportsShotVideoThumbnailSnapshot',
      'getShotVideoThumbnailUrl',
      'handleShotVideoThumbnailError',
      'version-video-thumbnail',
      'thumbnail-image',
      'thumbnail-fallback',
      'play-overlay',
      'x-oss-process=video/snapshot',
      '@error.stop="handleShotVideoThumbnailError(version.videoUrl)"',
      '@click.stop="handleOpenShotVideoPreview(shot, version)"',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('keeps video version cards compact with liuguang-style overlay controls', () => {
    const source = workbench();

    for (const token of [
      'version-status-row',
      'version-resolution',
      'detail-view-btn',
      'status-tag',
      'status-tag--backup',
      'status-tag--extract',
      'action-tag',
      'title="查看视频生成详情"',
      'title="抽帧"',
      '{{ isShotVideoVersionBackup(version) ? \'备选\' : \'备\' }}',
      '@click.stop="handleOpenVideoGenerationDetail(shot, version)"',
      '@click.stop="handleExtractShotVideoFrame(shot, version)"',
    ]) {
      expect(source).toContain(token);
    }

    expect(source).toContain('.version-actions.is-secondary');
    expect(source).toContain('grid-template-columns: repeat(2, minmax(0, 1fr));');
  });

  it('can create and show liuguang-style subtitle-removed video versions', () => {
    const source = workbench();

    for (const token of [
      'subtitleRemove',
      'removeShotVideoVersionSubtitleApi',
      'isShotVideoVersionSubtitleRemoved',
      'canRemoveShotVideoVersionSubtitle',
      'handleRemoveShotVideoVersionSubtitle',
      'status-tag--subtitle',
      '已擦字幕',
      '无字幕',
      '擦',
      'title="擦除字幕"',
      '@click.stop="handleRemoveShotVideoVersionSubtitle(shot, version)"',
      '@click="handleRemoveSubtitleVideoGenerationDetailVersion"',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('can create and show liuguang-style upscaled video versions', () => {
    const source = workbench();

    for (const token of [
      'upscaledFlag',
      'upscaledResolution',
      'upscaleShotVideoVersionApi',
      'upscaleResolutionOptions',
      'isShotVideoVersionUpscaled',
      'canUpscaleShotVideoVersion',
      'handleUpscaleShotVideoVersion',
      'handleUpscaleVideoGenerationDetailVersion',
      'status-tag--upscale',
      '已超分',
      '超分辨率',
      '超',
      '@click.stop="handleUpscaleShotVideoVersion(shot, version, option.value)"',
      '@click="handleUpscaleVideoGenerationDetailVersion(option.value)"',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('keeps project-mode and short-film-mode entry points in the production workbench', () => {
    const source = workbench();

    for (const token of [
      'workbenchMode',
      'workbenchModeOptions',
      '项目制',
      '短片制',
      '项目制生产流',
      '短片制工作台占位',
      '短片制能力正在接入',
    ]) {
      expect(source).toContain(token);
    }
  });
});
