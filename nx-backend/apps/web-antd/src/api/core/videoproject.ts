import type { VideoGeneration } from './video';

import { requestClient } from '#/api/request';

// ============ 类型定义（与后端 internal/videoproject JSON tags 保持一致：camelCase） ============

export interface Project {
  characterCount: number;
  completedShots: number;
  composeStatus: 'pending' | 'composing' | 'completed' | 'failed' | string;
  createTime: string;
  description: string;
  finalVideoAssetId: string;
  finalVideoInputHash: string;
  finalVideoUrl: string;
  id: string;
  name: string;
  scriptContent: string;
  scriptRevision: number;
  sceneCount: number;
  status: string;
  styleGuide: string;
  theme: string;
  totalShots: number;
  updateTime: string;
}

export interface Character {
  assetId: string;
  createTime: string;
  description: string;
  id: string;
  isMain: boolean;
  name: string;
  projectId: string;
  referenceImageUrl: string;
}

export interface Scene {
  assetId: string;
  createTime: string;
  description: string;
  id: string;
  name: string;
  projectId: string;
  referenceImageUrl: string;
  referenceVideoUrl: string;
}

export interface Shot {
  actionDescription: string;
  aspectRatio: '16:9' | '9:16' | '1:1' | string;
  cameraMovement: string;
  characterIds: string[];
  createTime: string;
  duration: 5 | 10 | 15 | number;
  dynamicDescription: string;
  endFrameUrl: string;
  errorMessage: string;
  generatedPrompt: string;
  generationId: string;
  generationRevision: number;
  gridStoryboardPrompt: string;
  id: string;
  imageReferenceModes: string[];
  name: string;
  orderNum: number;
  projectId: string;
  scriptOriginalContent: string;
  sceneId: string;
  selectedGenerationId: string;
  selectedGenerationRevision: number;
  selectedGenerationStatus: string;
  shotAssets: ShotAsset[];
  soundAndPictureTogether: string;
  sourceKey: string;
  sourceScriptRevision: number;
  status: 'draft' | 'generating' | 'completed' | 'failed' | string;
  storyboardUrl: string;
  updateTime: string;
  usedAudios: string[];
  usedImages: string[];
  usedVideos: string[];
  videoModel: string;
  videoReferenceMode: string;
  videoResolution: string;
  videoUrl: string;
}

export interface ShotAsset {
  assetType: 'audio' | 'image' | 'video' | string;
  createTime: string;
  id: string;
  mimeType: string;
  name: string;
  objectUrl: string;
  shotId: string;
  sizeBytes: number;
  sortOrder: number;
  updateTime: string;
}

export interface ShotVideoVersion {
  aspectRatio: string;
  backupFlag: boolean;
  createTime: string;
  errorMessage: string;
  id: string;
  isCurrent: boolean;
  model: string;
  prompt: string;
  seconds: number;
  shotId: string;
  shotRevision: number;
  status: string;
  subtitleRemove: string;
  updateTime: string;
  upscaledFlag: boolean;
  upscaledResolution: string;
  videoAssetId: string;
  videoUrl: string;
  viewedFlag: boolean;
}

export interface ShotVideoVersionDetailReference {
  label: string;
  type: 'audio' | 'image' | 'video' | string;
  url: string;
}

export interface ShotVideoVersionDetail {
  references: ShotVideoVersionDetailReference[];
  shot: Shot;
  version: ShotVideoVersion;
}

export interface ShotPreview {
  audios: string[];
  estimatedSuccessRate: number;
  images: string[];
  prompt: string;
  validation: {
    errors: string[];
    isValid: boolean;
    warnings: string[];
  };
  videos: string[];
}

export interface PageResult<T> {
  items: T[];
  total: number;
}

export interface BatchGenerateResponse {
  failedCount: number;
  projectId: string;
  shotResults: Array<{
    errorMessage: string;
    generationId: string;
    orderNum: number;
    shotId: string;
    shotName: string;
    status: 'success' | 'failed' | 'skipped' | string;
  }>;
  successCount: number;
  totalShots: number;
}

export interface BatchProgressResponse {
  completed: number;
  failed: number;
  generating: number;
  pending: number;
  progress: number;
  projectId: string;
  total: number;
}

export interface ComposeProjectInput {
  enableSubtitles?: boolean;
  excludedShotIds?: string[];
  musicUrl?: string;
  partialAcknowledged?: boolean;
  transition?: string;
}

export interface ComposeVideoResponse {
  error?: string;
  inputHash: string;
  isCurrent: boolean;
  jobId: string;
  progress: number;
  projectId: string;
  status: 'queued' | 'processing' | 'completed' | 'failed' | string;
  videoUrl: string;
}

export interface ComposeStatusResponse {
  canCompose: boolean;
  completedShots: number;
  composeStatus: 'pending' | 'composing' | 'completed' | 'failed' | string;
  finalVideoUrl: string;
  projectId: string;
  totalShots: number;
}

export type WorkflowStepKey = 'assets' | 'brief' | 'export' | 'generate' | 'storyboard';
export type WorkflowStepState = 'blocked' | 'complete' | 'optional' | 'skipped_existing' | 'stale';
export type ShotReadiness = 'completed' | 'failed' | 'generating' | 'incomplete' | 'ready' | 'recovery' | 'stale';

export interface WorkflowShotStatus {
  canGenerate: boolean;
  readiness: ShotReadiness;
  shot: Shot;
}

export interface ProjectWorkflow {
  project: Project;
  recommendedStep: WorkflowStepKey;
  shots: WorkflowShotStatus[];
  steps: Record<WorkflowStepKey, WorkflowStepState>;
}

export interface GenerationSubmission {
  error?: string;
  generationId?: string;
  requestKey: string;
  shotId: string;
  status: string;
  submissionId: number;
  taskId?: string;
}

export interface ScriptParagraph {
  content: string;
  index: number;
}

export interface ScriptImportItem {
  error?: string;
  index: number;
  shotId?: string;
  sourceKey: string;
  status: 'created' | 'existing' | 'failed' | 'pending';
}

export interface ScriptImportResult {
  created: ScriptImportItem[];
  existing: ScriptImportItem[];
  failed: ScriptImportItem[];
  items: ScriptImportItem[];
}

// ============ 项目 API ============

export function createProjectApi(data: Partial<Project>) {
  return requestClient.post<Project>('/video/projects', data);
}

export function listProjectsApi(params?: Record<string, any>) {
  return requestClient.get<PageResult<Project>>('/video/projects/list', {
    params,
  });
}

export function getProjectApi(id: string | number) {
  return requestClient.get<Project>(`/video/projects/${id}`);
}

export function updateProjectApi(id: string | number, data: Partial<Project>) {
  return requestClient.put<Project>(`/video/projects/${id}`, data);
}

export function deleteProjectApi(id: string | number) {
  return requestClient.delete(`/video/projects/${id}`);
}

// ============ 角色 API ============

export function createCharacterApi(
  projectId: string | number,
  data: Partial<Character>,
) {
  return requestClient.post<Character>(
    `/video/projects-characters/${projectId}`,
    data,
  );
}

export function listCharactersApi(projectId: string | number) {
  return requestClient.get<Character[]>(`/video/projects-characters/${projectId}`);
}

export function updateCharacterApi(id: string | number, data: Partial<Character>) {
  return requestClient.put<Character>(`/video/projects-characters/_/${id}`, data);
}

export function deleteCharacterApi(id: string | number) {
  return requestClient.delete(`/video/projects-characters/_/${id}`);
}

// ============ 场景 API ============

export function createSceneApi(projectId: string | number, data: Partial<Scene>) {
  return requestClient.post<Scene>(`/video/projects-scenes/${projectId}`, data);
}

export function listScenesApi(projectId: string | number) {
  return requestClient.get<Scene[]>(`/video/projects-scenes/${projectId}`);
}

export function updateSceneApi(id: string | number, data: Partial<Scene>) {
  return requestClient.put<Scene>(`/video/projects-scenes/_/${id}`, data);
}

export function deleteSceneApi(id: string | number) {
  return requestClient.delete(`/video/projects-scenes/_/${id}`);
}

// ============ 分镜 API ============

export function createShotApi(projectId: string | number, data: Partial<Shot>) {
  return requestClient.post<Shot>(`/video/projects-shots/${projectId}`, data);
}

export function listShotsApi(projectId: string | number) {
  return requestClient.get<Shot[]>(`/video/projects-shots/list/${projectId}`);
}

export function getShotApi(id: string | number) {
  return requestClient.get<Shot>(`/video/shots/${id}`);
}

export function updateShotApi(id: string | number, data: Partial<Shot>) {
  return requestClient.put<Shot>(`/video/shots/${id}`, data);
}

export function deleteShotApi(id: string | number) {
  return requestClient.delete(`/video/shots/${id}`);
}

export function listShotAssetsApi(shotId: string | number) {
  return requestClient.get<ShotAsset[]>(`/video/shots-assets/list/${shotId}`);
}

export function createShotAssetApi(
  shotId: string | number,
  data: Partial<ShotAsset>,
) {
  return requestClient.post<ShotAsset>(`/video/shots-assets/${shotId}`, data);
}

export function deleteShotAssetApi(id: string | number) {
  return requestClient.delete(`/video/shots-assets/${id}`);
}

export function listShotVideoVersionsApi(shotId: string | number) {
  return requestClient.get<ShotVideoVersion[]>(
    `/video/shots-video-versions/list/${shotId}`,
  );
}

export function getShotVideoVersionDetailApi(
  shotId: string | number,
  generationId: string | number,
) {
  return requestClient.get<ShotVideoVersionDetail>(
    `/video/shots-video-versions/detail/${shotId}/${generationId}`,
  );
}

export function setShotVideoVersionApi(
  shotId: string | number,
  generationId: string | number,
) {
  return requestClient.post<Shot>(
    `/video/shots-video-versions/set/${shotId}/${generationId}`,
  );
}

export function setShotVideoVersionBackupApi(
  shotId: string | number,
  generationId: string | number,
  backupFlag: boolean,
) {
  return requestClient.post<ShotVideoVersion>(
    `/video/shots-video-versions/backup/${shotId}/${generationId}`,
    { backupFlag },
  );
}

export function removeShotVideoVersionSubtitleApi(
  shotId: string | number,
  generationId: string | number,
) {
  return requestClient.post<ShotVideoVersion>(
    `/video/shots-video-versions/remove-subtitle/${shotId}/${generationId}`,
  );
}

export function upscaleShotVideoVersionApi(
  shotId: string | number,
  generationId: string | number,
  data: { resolution: string },
) {
  return requestClient.post<ShotVideoVersion>(
    `/video/shots-video-versions/upscale/${shotId}/${generationId}`,
    data,
  );
}

export function refreshShotVideoVersionsApi(shotId: string | number) {
  return requestClient.post<ShotVideoVersion[]>(
    `/video/shots-video-versions/refresh/${shotId}`,
  );
}

export function copyShotVideoVersionApi(
  sourceShotId: string | number,
  generationId: string | number,
  targetShotId: string | number,
) {
  return requestClient.post<Shot>(
    `/video/shots-video-versions/copy/${sourceShotId}/${generationId}/${targetShotId}`,
  );
}

export function extractShotVideoFrameApi(
  shotId: string | number,
  generationId: string | number,
) {
  return requestClient.post<ShotAsset>(
    `/video/shots-video-versions/extract-frame/${shotId}/${generationId}`,
  );
}

export function deleteShotVideoVersionApi(
  shotId: string | number,
  generationId: string | number,
) {
  return requestClient.delete(
    `/video/shots-video-versions/${shotId}/${generationId}`,
  );
}

export function markShotVideoVersionViewedApi(generationId: string | number) {
  return requestClient.post(`/video/shots-video-versions/viewed/${generationId}`);
}

export function generateShotApi(
  shotId: string | number,
  input: { requestKey: string },
) {
  return requestClient.post<VideoGeneration>(`/video/shots-generate/${shotId}`, input);
}

export function previewShotPromptApi(shotId: string | number) {
  return requestClient.get<ShotPreview>(`/video/shots-preview/${shotId}`);
}

// ============ 批量生成和视频合成 API ============

export function batchGenerateShotsApi(
  projectId: string | number,
  input: { items: Array<{ requestKey: string; shotId: string }> },
) {
  return requestClient.post<BatchGenerateResponse>(
    `/video/projects-batch-generate/${projectId}`,
    input,
    { timeout: 180_000 },
  );
}

export function getBatchProgressApi(projectId: string | number) {
  return requestClient.get<BatchProgressResponse>(
    `/video/projects-batch-progress/${projectId}`,
  );
}

export function composeProjectVideoApi(
  projectId: string | number,
  data: ComposeProjectInput = {},
) {
  return requestClient.post<ComposeVideoResponse>(
    `/video/projects-compose/${projectId}`,
    data,
    { timeout: 180_000 },
  );
}

export function getComposeStatusApi(projectId: string | number) {
  return requestClient.get<ComposeStatusResponse>(
    `/video/projects-compose-status/${projectId}`,
  );
}

export function getComposeJobApi(projectId: string | number, jobId: string | number) {
  return requestClient.get<ComposeVideoResponse>(
    `/video/projects-compose-safe-status/${projectId}/${jobId}`,
  );
}

export function getProjectWorkflowApi(projectId: string | number) {
  return requestClient.get<ProjectWorkflow>(`/video/projects-workflow/${projectId}`);
}

export function getGenerationSubmissionApi(submissionId: string | number) {
  return requestClient.get<GenerationSubmission>(
    `/video/generation-submissions/${submissionId}`,
  );
}

export function createShotsFromScriptApi(
  projectId: string | number,
  input: { items: ScriptParagraph[]; scriptRevision: number },
) {
  return requestClient.post<ScriptImportResult>(
    `/video/projects-shots/from-script/${projectId}`,
    input,
  );
}

export function generateShotSafeApi(
  shotId: string | number,
  input: { requestKey: string },
) {
  return requestClient.post<VideoGeneration>(
    `/video/shots-generate-safe/${shotId}`,
    input,
  );
}

export function batchGenerateShotsSafeApi(
  projectId: string | number,
  input: { items: Array<{ requestKey: string; shotId: string }> },
) {
  return requestClient.post<BatchGenerateResponse>(
    `/video/projects-batch-generate-safe/${projectId}`,
    input,
    { timeout: 180_000 },
  );
}

export function reconcileGenerationSubmissionApi(
  requestKey: string,
  input: { taskId: string },
) {
  return requestClient.post<VideoGeneration>(
    `/video/generation-submissions/reconcile/${requestKey}`,
    input,
  );
}

export function composeProjectSafeApi(
  projectId: string | number,
  input: ComposeProjectInput,
) {
  return requestClient.post<ComposeVideoResponse>(
    `/video/projects-compose-safe/${projectId}`,
    input,
  );
}

// ============ 兼容旧页面命名 ============
export type VideoProject = Project;
export const createVideoProjectApi = createProjectApi;
export const deleteVideoProjectApi = deleteProjectApi;
export const listVideoProjectsApi = listProjectsApi;
export const updateVideoProjectApi = updateProjectApi;
