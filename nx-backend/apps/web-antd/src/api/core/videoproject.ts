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
  finalVideoUrl: string;
  id: string;
  name: string;
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
  endFrameUrl: string;
  errorMessage: string;
  generatedPrompt: string;
  generationId: string;
  id: string;
  imageReferenceModes: string[];
  name: string;
  orderNum: number;
  projectId: string;
  sceneId: string;
  status: 'draft' | 'generating' | 'completed' | 'failed' | string;
  updateTime: string;
  usedImages: string[];
  usedVideos: string[];
  videoReferenceMode: string;
  videoUrl: string;
}

export interface ShotPreview {
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
  musicUrl?: string;
  transition?: string;
}

export interface ComposeVideoResponse {
  duration: number;
  errorMessage?: string;
  fileSize: number;
  projectId: string;
  shotCount: number;
  status: 'completed' | 'failed' | string;
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

export function generateShotApi(shotId: string | number) {
  return requestClient.post<VideoGeneration>(`/video/shots-generate/${shotId}`, {});
}

export function previewShotPromptApi(shotId: string | number) {
  return requestClient.get<ShotPreview>(`/video/shots-preview/${shotId}`);
}

// ============ 批量生成和视频合成 API ============

export function batchGenerateShotsApi(projectId: string | number) {
  return requestClient.post<BatchGenerateResponse>(
    `/video/projects-batch-generate/${projectId}`,
    {},
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

// ============ 兼容旧页面命名 ============
export type VideoProject = Project;
export const createVideoProjectApi = createProjectApi;
export const deleteVideoProjectApi = deleteProjectApi;
export const listVideoProjectsApi = listProjectsApi;
export const updateVideoProjectApi = updateProjectApi;
