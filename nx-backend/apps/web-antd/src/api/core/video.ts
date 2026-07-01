import type { PageResult } from './signup';

import { requestClient } from '#/api/request';

export interface VideoGeneration {
  aspectRatio: string;
  createTime: string;
  duration: number;
  errorMessage: string;
  fps: number;
  height: number;
  id: string;
  imageUrl: string;
  model: string;
  prompt: string;
  provider: string;
  seconds: number;
  status: string;
  taskId: string;
  updateTime: string;
  videoAssetId: string;
  videoUrl: string;
  width: number;
}

export interface VideoAnalysisJob {
  assets: string[];
  audioSummary: string;
  characters: string[];
  createTime: string;
  errorMessage: string;
  hasSpeech: boolean;
  id: string;
  rawResult: string;
  scenes: string[];
  seedancePrompt: string;
  speechKeywords: string[];
  speechOutline: string[];
  speechTopics: string[];
  status: string;
  updateTime: string;
  videoAssetId: string;
  videoName: string;
  videoUrl: string;
}

export interface VideoStoryboardShot {
  action: string;
  assets: string[];
  audio: string;
  camera: string;
  characters: string[];
  composition: string;
  dialogue: string;
  duration: number;
  index: number;
  lighting: string;
  scene: string;
  seedancePrompt: string;
  title: string;
}

export interface VideoStoryboard {
  analysisJobId: string;
  createTime: string;
  errorMessage: string;
  globalPrompt: string;
  id: string;
  rawResult: string;
  shots: VideoStoryboardShot[];
  status: string;
  styleGuide: string[];
  theme: string;
  title: string;
  updateTime: string;
}

export function generateVideoApi(data: {
  aspectRatio?: string;
  audios?: string[];
  images?: string[];
  imageUrl?: string;
  model?: string;
  prompt: string;
  seconds?: number;
  videos?: string[];
}) {
  return requestClient.post<VideoGeneration>('/video/generate', data, {
    timeout: 180_000,
  });
}

export function getVideoGenerationsApi(params?: Record<string, any>) {
  return requestClient.get<PageResult<VideoGeneration>>(
    '/video/generations/list',
    { params },
  );
}

export function getVideoGenerationApi(id: string) {
  return requestClient.get<VideoGeneration>(`/video/generations/${id}`);
}

export function refreshVideoGenerationApi(id: string) {
  return requestClient.post<VideoGeneration>(
    `/video/generations/${id}`,
    undefined,
    { timeout: 60_000 },
  );
}

export function createVideoAnalysisApi(data: {
  videoAssetId?: string;
  videoName?: string;
  videoUrl: string;
}) {
  return requestClient.post<VideoAnalysisJob>('/video/analysis', data);
}

export function getVideoAnalysisJobsApi(params?: Record<string, any>) {
  return requestClient.get<PageResult<VideoAnalysisJob>>(
    '/video/analysis/list',
    { params },
  );
}

export function retryVideoAnalysisApi(id: string) {
  return requestClient.post<VideoAnalysisJob>(
    `/video/analysis/${id}/retry`,
    undefined,
    { timeout: 30_000 },
  );
}

export function createVideoStoryboardApi(data: {
  analysisJobId: string;
  theme: string;
  title?: string;
}) {
  return requestClient.post<VideoStoryboard>('/video/storyboards', data, {
    timeout: 30_000,
  });
}

export function getVideoStoryboardsApi(params?: Record<string, any>) {
  return requestClient.get<PageResult<VideoStoryboard>>(
    '/video/storyboards/list',
    { params },
  );
}

export function getVideoStoryboardApi(id: string) {
  return requestClient.get<VideoStoryboard>(`/video/storyboards/${id}`);
}

export function updateVideoStoryboardApi(
  id: string,
  data: {
    globalPrompt: string;
    shots: VideoStoryboardShot[];
    styleGuide: string[];
    theme: string;
    title: string;
  },
) {
  return requestClient.put<VideoStoryboard>(`/video/storyboards/${id}`, data);
}

export function retryVideoStoryboardApi(id: string) {
  return requestClient.post<VideoStoryboard>(
    `/video/storyboards/${id}/retry`,
    undefined,
    { timeout: 30_000 },
  );
}

export function deleteVideoStoryboardApi(id: string) {
  return requestClient.delete<boolean>(`/video/storyboards/${id}`);
}
