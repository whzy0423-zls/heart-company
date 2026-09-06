import { requestClient } from '#/api/request';

export interface StoryGenerationConfigView {
  enabled: boolean;
  provider: string;
  apiBase: string;
  apiKeySet: boolean;
  model: string;
  temperature: number;
  maxTokens: number;
  timeoutSeconds: number;
  systemPrompt: string;
}

export interface StoryGenerationConfigPayload {
  enabled: boolean;
  provider: string;
  apiBase: string;
  apiKey: string;
  model: string;
  temperature: number;
  maxTokens: number;
  timeoutSeconds: number;
  systemPrompt: string;
}

export interface StoryGenerationPingResult {
  ok: boolean;
  message: string;
  latencyMs?: number;
  apiBase?: string;
  model?: string;
}

export function getStoryGenerationConfigApi() {
  return requestClient.get<StoryGenerationConfigView>('/story-generation-config');
}

export function updateStoryGenerationConfigApi(data: Partial<StoryGenerationConfigPayload>) {
  return requestClient.put<StoryGenerationConfigView>('/story-generation-config', {
    storyGeneration: data,
  });
}

export function testStoryGenerationConfigApi(data: Partial<StoryGenerationConfigPayload>) {
  return requestClient.post<StoryGenerationPingResult>('/story-generation-config/test', { storyGeneration: data });
}
