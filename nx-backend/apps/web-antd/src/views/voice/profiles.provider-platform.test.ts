import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

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
  it('defaults new voice profiles to Aliyun Bailian while retaining MiniMax', () => {
    expect(source).toContain('const voiceProviderOptions');
    expect(source).toContain("provider: 'bailian'");
    expect(source).toContain("{ label: '阿里百炼（推荐）', value: 'bailian' }");
    expect(source).toContain(
      "{ label: 'MiniMax（旧流程）', value: 'minimax' }",
    );
    expect(source).toContain('v-model:value="form.provider"');
    expect(source).toContain('provider: form.provider');
    expect(source).toContain('百炼复刻需上传后的对象存储公网 URL');
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

  it('switches synthesis models when a Bailian voice is selected', () => {
    for (const pageSource of [testPageSource, contentPageSource]) {
      expect(pageSource).toContain("provider === 'bailian'");
      expect(pageSource).toContain('MiniMax/speech-2.8-turbo');
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
    expect(source).toContain('复制到百炼');
  });

  it('confirms that the MiniMax voice and original sample are retained before copying', () => {
    expect(source).toContain("title: '复制到百炼'");
    expect(source).toContain('保留原 MiniMax 音色');
    expect(source).toContain('复用原音频样本');
  });

  it('tracks copy loading per row, refreshes after the API call, and guides successful selection', () => {
    expect(source).toContain(
      'const copyingProfileId = ref<null | string>(null)',
    );
    expect(source).toContain(':loading="copyingProfileId === record.id"');
    expect(source).toContain('await copyVoiceProfileToBailianApi(record.id)');
    expect(source).toContain('await load()');
    expect(source).toContain('已复制到百炼，可到芯之力模型配置选择');
  });

  it('surfaces the backend copy error message', () => {
    expect(source).toContain('error?.response?.data?.error');
    expect(source).toContain('error?.response?.data?.message');
    expect(source).toContain('复制到百炼失败，请稍后重试');
  });
});
