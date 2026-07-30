import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

import {
  getBailianCopyFeedback,
  updateCopyingProfileIds,
} from './profiles-copy-feedback';

const source = readFileSync(resolve(__dirname, 'profiles.vue'), 'utf8');
const apiSource = readFileSync(
  resolve(__dirname, '../../api/core/voice.ts'),
  'utf8',
);
const testPageSource = readFileSync(resolve(__dirname, 'test.vue'), 'utf8');
const contentPageSource = readFileSync(
  resolve(__dirname, 'content.vue'),
  'utf8',
);
const bailianCopyApiSource = apiSource.slice(
  apiSource.indexOf('export function copyVoiceProfileToBailianApi'),
  apiSource.indexOf('export function deleteVoiceProfileApi'),
);

describe('voice profile clone provider platform', () => {
  it('creates all new voice profiles with Aliyun Bailian Qwen', () => {
    expect(source).toContain("provider: 'bailian'");
    expect(source).not.toContain('const voiceProviderOptions');
    expect(source).not.toContain("value: 'minimax'");
    expect(source).not.toContain('v-model:value="form.provider"');
    expect(source).toContain('provider: form.provider');
    expect(source).toContain('阿里百炼 Qwen 声音复刻');
    expect(source).toContain('qwen3-tts-vc-2026-01-22');
    expect(source).toContain('先在芯之力模型配置中保存百炼 API Key');
  });

  it('shows the clone platform in the profile list', () => {
    expect(source).toContain("dataIndex: 'provider'");
    expect(source).toContain('platformLabel(record.provider)');
    expect(source).toContain('platformColor(record.provider)');
  });

  it('extends voice options with provider for same-provider reuse', () => {
    const optionType = apiSource.slice(
      apiSource.indexOf('export interface VoiceOption'),
      apiSource.indexOf('}', apiSource.indexOf('export interface VoiceOption')),
    );
    expect(optionType).toContain('provider: string');
  });

  it('switches synthesis models when a Bailian Qwen voice is selected', () => {
    for (const pageSource of [testPageSource, contentPageSource]) {
      expect(pageSource).toContain("provider === 'bailian'");
      expect(pageSource).toContain('qwen3-tts-vc-2026-01-22');
      expect(pageSource).toContain("'speech-02-hd'");
      expect(pageSource).toContain('watch(');
    }
  });
});

describe('copy MiniMax profile to Bailian', () => {
  it('posts the selected profile to the Bailian-copy endpoint with the clone timeout', () => {
    expect(bailianCopyApiSource).toContain(
      'export function copyVoiceProfileToBailianApi(id: string)',
    );
    expect(bailianCopyApiSource).toContain(
      '`/voice/profiles/${id}/copy-to-bailian`',
    );
    expect(bailianCopyApiSource).toContain('timeout: 180_000');
  });

  it('only exposes the Bailian-copy action for MiniMax profiles with a sample asset', () => {
    expect(source).toContain(
      "record.provider === 'minimax' && record.sampleAssetId",
    );
    expect(source).toContain("record.status !== 'migrated'");
    expect(source).toContain('迁移到百炼 Qwen');
  });

  it('confirms migration to Qwen and deactivation of the old MiniMax profile', () => {
    expect(source).toContain("title: '迁移到百炼 Qwen'");
    expect(source).toContain('迁移成功后停用原 MiniMax 音色');
    expect(source).toContain('复用原音频样本');
    expect(source).toContain("['draft', 'failed'].includes(record.status)");
  });

  it('uses a per-profile Set for concurrent copy loading and refreshes after the API call', () => {
    expect(source).toContain(
      'const copyingProfileIds = ref(new Set<string>())',
    );
    expect(source).toContain(':loading="isCopyingProfile(record.id)"');
    expect(source).toContain('await copyVoiceProfileToBailianApi(record.id)');
    expect(source).toContain('await load()');
  });

  it('delegates rejected copy errors to the shared request interceptor', () => {
    const copyHandlerSource = source.slice(
      source.indexOf('function copyProfileToBailian'),
      source.indexOf('function profileRecord'),
    );
    expect(copyHandlerSource).not.toContain('message.error');
    expect(copyHandlerSource).toContain('catch {');
  });
});

describe('Bailian copy feedback', () => {
  it.each([
    ['ready', '', 'success', '已迁移到百炼 Qwen，可到芯之力模型配置选择'],
    ['cloning', '', 'info', '已提交百炼 Qwen 迁移，正在处理中，请稍后刷新查看状态'],
    ['draft', '', 'info', '已提交百炼 Qwen 迁移，正在处理中，请稍后刷新查看状态'],
    ['failed', '百炼服务暂不可用', 'error', '百炼服务暂不可用'],
  ])(
    'maps %s responses to accurate user feedback',
    (status, lastError, type, content) => {
      expect(getBailianCopyFeedback({ lastError, status })).toEqual({
        content,
        type,
      });
    },
  );

  it('removes only the completed profile from concurrent copy loading', () => {
    const copying = updateCopyingProfileIds(new Set(), 'minimax-1', true);
    const concurrentCopying = updateCopyingProfileIds(
      copying,
      'minimax-2',
      true,
    );

    expect(
      updateCopyingProfileIds(concurrentCopying, 'minimax-1', false),
    ).toEqual(new Set(['minimax-2']));
  });
});
