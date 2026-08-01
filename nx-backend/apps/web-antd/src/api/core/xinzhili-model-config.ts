import type { VoiceOption } from './voice';
import { requestClient } from '#/api/request';

export type XinzhiliMode =
  | 'argument'
  | 'comfort'
  | 'deep_listening'
  | 'normal';

export interface XinzhiliTimingConfig {
  argumentCandidateSilenceMs: number;
  comfortEndSilenceMs: number;
  comfortFirstPromptMs: number;
  comfortSecondPromptMs: number;
  deepListeningEndSilenceMs: number;
  deepListeningPromptMs: number;
  maxProactivePrompts: number;
  normalEndSilenceMs: number;
  partialStableMs: number;
}

interface XinzhiliSecretView {
  apiKey: string;
  apiKeySet: boolean;
  apiKeySuffix: string;
}

export interface XinzhiliModelConfigView {
  commonPrompt: string;
  enabled: boolean;
  enabledModes: XinzhiliMode[];
  modePrompts: Partial<Record<XinzhiliMode, string>>;
  realtimeAsr: XinzhiliSecretView & {
    endpoint: string;
    model: 'paraformer-realtime-v2';
    provider: 'aliyun-bailian';
    region: string;
  };
  timing: XinzhiliTimingConfig;
  tts: XinzhiliSecretView & {
    endpoint: string;
    format: 'mp3';
    groupId?: string;
    model: string;
    provider: 'bailian' | 'minimax' | 'openai-compatible';
    voice: string;
  };
  version: number;
}

export interface XinzhiliModelConfigPayload {
  commonPrompt: string;
  enabled: boolean;
  enabledModes: XinzhiliMode[];
  expectedVersion: number;
  modePrompts: Partial<Record<XinzhiliMode, string>>;
  realtimeAsr: {
    apiKey: string;
    endpoint: string;
    model: 'paraformer-realtime-v2';
    provider: 'aliyun-bailian';
    region: string;
  };
  timing: XinzhiliTimingConfig;
  tts: {
    apiKey: string;
    endpoint: string;
    format: 'mp3';
    groupId?: string;
    model: string;
    provider: 'bailian' | 'minimax' | 'openai-compatible';
    voice: string;
  };
}

export function getXinzhiliModelConfigApi() {
  return requestClient.get<XinzhiliModelConfigView>(
    '/xinzhili-model-config',
  );
}

export function updateXinzhiliModelConfigApi(
  data: XinzhiliModelConfigPayload,
) {
  return requestClient.put<XinzhiliModelConfigView>(
    '/xinzhili-model-config',
    data,
  );
}

export function getXinzhiliVoiceOptionsApi() {
  return requestClient.get<VoiceOption[]>('/voice/options');
}
