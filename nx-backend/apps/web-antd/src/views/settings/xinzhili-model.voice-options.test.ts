import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const viewSource = readFileSync(resolve(__dirname, 'xinzhili-model.vue'), 'utf8');
const apiSource = readFileSync(
  resolve(__dirname, '../../api/core/xinzhili-model-config.ts'),
  'utf8',
);

describe('xinzhili model voice option reuse', () => {
  it('reuses the existing voice option library for cloned voices', () => {
    expect(apiSource).toContain("from './voice'");
    expect(apiSource).toContain('getXinzhiliVoiceOptionsApi');
    expect(apiSource).toContain("'/voice/options'");

    expect(viewSource).toContain('getXinzhiliVoiceOptionsApi');
    expect(viewSource).toContain('voiceOptions');
    expect(viewSource).toContain('可直接复用声音管理里的已克隆音色');
    expect(viewSource).toContain('选择已有音色');
    expect(viewSource).toContain('applyVoiceOption');
    expect(viewSource).toContain("option.source === 'clone'");
    expect(viewSource).toContain('form.value.tts.voice = option.voiceId');
  });

  it('keeps manual voice id editing as a fallback', () => {
    expect(viewSource).toContain('也可以手动填写平台音色 ID');
    expect(viewSource).toContain('form.tts.voice');
  });
});
