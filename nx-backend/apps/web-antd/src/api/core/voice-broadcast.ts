import { requestClient } from '#/api/request';

export interface VoiceBroadcastVoice {
  id: string;
  name: string;
  provider: string;
  model: string;
}

export interface VoiceBroadcastHealth {
  errorCategory?: string;
  latencyMs: number;
  message?: string;
  ok: boolean;
  status: 'error' | 'not_configured' | 'ok' | 'unknown' | string;
  testedAt?: string;
}

export interface VoiceBroadcastConfigView {
  apiKeySet: boolean;
  apiKeySuffix?: string;
  currentVoice: string;
  defaultVoice: string;
  enabled: boolean;
  health: VoiceBroadcastHealth;
  model: string;
  provider: string;
  region: string;
  version: number;
  voices: VoiceBroadcastVoice[];
  workspaceId: string;
  updatedAt?: string;
}

export interface VoiceBroadcastConfigPayload {
  apiKey?: string;
  clearApiKey?: boolean;
  currentVoice: string;
  defaultVoice: string;
  enabled: boolean;
  expectedVersion: number;
  model: string;
  provider: string;
  region: string;
  voices?: VoiceBroadcastVoice[];
  workspaceId: string;
}

export type VoiceBroadcastTestPayload = Partial<
  Omit<VoiceBroadcastConfigPayload, 'expectedVersion'>
> & {
  expectedVersion?: number;
  text?: string;
};

export function getVoiceBroadcastConfigApi() {
  return requestClient.get<VoiceBroadcastConfigView>(
    '/app/voice-broadcast-config',
  );
}

export function updateVoiceBroadcastConfigApi(
  data: VoiceBroadcastConfigPayload,
) {
  return requestClient.put<VoiceBroadcastConfigView>(
    '/app/voice-broadcast-config',
    data,
  );
}

export function testVoiceBroadcastConfigApi(data: VoiceBroadcastTestPayload) {
  return requestClient.post<VoiceBroadcastConfigView>(
    '/app/voice-broadcast-config/test',
    data,
    { timeout: 30_000 },
  );
}
