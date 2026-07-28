import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const source = readFileSync(resolve(__dirname, 'profiles.vue'), 'utf8');
const apiSource = readFileSync(
  resolve(__dirname, '../../api/core/voice.ts'),
  'utf8',
);

describe('voice profile clone provider platform', () => {
  it('defaults new voice profiles to Aliyun Bailian while retaining MiniMax', () => {
    expect(source).toContain('const voiceProviderOptions');
    expect(source).toContain("provider: 'bailian'");
    expect(source).toContain("{ label: '阿里百炼（推荐）', value: 'bailian' }");
    expect(source).toContain("{ label: 'MiniMax（旧流程）', value: 'minimax' }");
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
});
