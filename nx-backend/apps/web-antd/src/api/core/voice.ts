import type { PageResult } from './signup';

import { requestClient } from '#/api/request';

export interface VoiceProfile {
  createTime: string;
  id: string;
  lastError: string;
  model: string;
  name: string;
  provider: string;
  remark: string;
  sampleAssetId: string;
  sampleName: string;
  sampleUrl: string;
  status: string;
  updateTime: string;
  voiceId: string;
}

export interface VoiceGeneration {
  audioAssetId: string;
  audioUrl: string;
  createTime: string;
  errorMessage: string;
  id: string;
  model: string;
  profileId: string;
  provider: string;
  status: string;
  text: string;
  voiceId: string;
}

export interface VoiceOption {
  id: string;
  label: string;
  model: string;
  provider: string;
  source: 'clone' | 'official';
  voiceId: string;
  voiceName: string;
}

export interface VoiceContentJob {
  audioAssetId: string;
  audioUrl: string;
  createTime: string;
  errorMessage: string;
  id: string;
  model: string;
  profileId: string;
  sourceAssetId: string;
  sourceName: string;
  sourceType: string;
  sourceUrl: string;
  status: string;
  text: string;
  title: string;
  voiceId: string;
  voiceName: string;
  voiceSource: 'clone' | 'official';
}

export function getVoiceProfilesApi(params?: Record<string, any>) {
  return requestClient.get<PageResult<VoiceProfile>>('/voice/profiles/list', {
    params,
  });
}

export function createVoiceProfileApi(data: {
  name: string;
  provider?: string;
  remark?: string;
  sampleAssetId: string;
  sampleName?: string;
  sampleUrl?: string;
  voiceId?: string;
}) {
  return requestClient.post<VoiceProfile>('/voice/profiles', data, {
    timeout: 180_000,
  });
}

export function cloneVoiceProfileApi(id: string) {
  return requestClient.post<VoiceProfile>(`/voice/profiles/${id}`, undefined, {
    timeout: 180_000,
  });
}

export function copyVoiceProfileToBailianApi(id: string) {
  return requestClient.post<VoiceProfile>(
    `/voice/profiles/${id}/copy-to-bailian`,
    undefined,
    { timeout: 180_000 },
  );
}

export function deleteVoiceProfileApi(id: string) {
  return requestClient.delete<boolean>(`/voice/profiles/${id}`);
}

export function generateVoiceApi(data: {
  model?: string;
  profileId: string;
  text: string;
  voiceId?: string;
}) {
  return requestClient.post<VoiceGeneration>('/voice/generate', data, {
    timeout: 180_000,
  });
}

export function getVoiceGenerationsApi(params?: Record<string, any>) {
  return requestClient.get<PageResult<VoiceGeneration>>(
    '/voice/generations/list',
    { params },
  );
}

export function getVoiceOptionsApi() {
  return requestClient.get<VoiceOption[]>('/voice/options');
}

export function generateVoiceContentApi(data: {
  model?: string;
  profileId?: string;
  sourceAssetId?: string;
  sourceName?: string;
  sourceType?: string;
  sourceUrl?: string;
  text: string;
  title: string;
  voiceId: string;
  voiceName?: string;
  voiceSource: 'clone' | 'official';
}) {
  return requestClient.post<VoiceContentJob>('/voice/content/generate', data, {
    timeout: 180_000,
  });
}

export function getVoiceContentJobsApi(params?: Record<string, any>) {
  return requestClient.get<PageResult<VoiceContentJob>>('/voice/content/list', {
    params,
  });
}

export type BailianCredentialSource =
  | 'legacy-asr'
  | 'legacy-tts'
  | 'none'
  | 'shared';

/** Safe credential status returned by the backend; the API key is never returned. */
export interface BailianCredentialView {
  apiKeySet: boolean;
  apiKeySuffix: string;
  source: BailianCredentialSource;
  version: number;
}

export interface UpdateBailianCredentialsPayload {
  apiKey: string;
  clearApiKey: boolean;
  expectedVersion: number;
}

export function getBailianCredentialsApi() {
  return requestClient.get<BailianCredentialView>('/voice/bailian-credentials');
}

export function updateBailianCredentialsApi(
  data: UpdateBailianCredentialsPayload,
) {
  return requestClient.put<BailianCredentialView>(
    '/voice/bailian-credentials',
    data,
  );
}
