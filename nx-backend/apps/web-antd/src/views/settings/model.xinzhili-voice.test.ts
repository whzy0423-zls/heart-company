import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const viewSource = readFileSync(resolve(__dirname, 'model.vue'), 'utf8');
const apiSource = readFileSync(
  resolve(__dirname, '../../api/core/model-config.ts'),
  'utf8',
);
const section = viewSource.slice(
  viewSource.indexOf('芯之力语音配置'),
  viewSource.indexOf('智能辅助作答'),
);

describe('xinzhili voice model form', () => {
  it('configures separate OpenAI-compatible ASR and TTS models', () => {
    expect(apiSource).toContain('xinzhiliVoice:');
    expect(section).toContain('form.xinzhiliVoice.asr.apiBase');
    expect(section).toContain('form.xinzhiliVoice.asr.model');
    expect(section).toContain('form.xinzhiliVoice.tts.apiBase');
    expect(section).toContain('form.xinzhiliVoice.tts.voice');
    expect(section).toContain('OpenAI 兼容协议');
    expect(section).not.toContain('Anthropic');
  });

  it('keeps speech keys masked and exposes interaction timing', () => {
    expect(viewSource).toContain('xinzhiliAsrKeySet');
    expect(viewSource).toContain('xinzhiliTtsKeySet');
    expect(section).toContain('form.xinzhiliVoice.interaction.endSilenceMs');
    expect(section).toContain('form.xinzhiliVoice.interaction.minSpeechMs');
    expect(section).toContain('form.xinzhiliVoice.interaction.maxTurnSeconds');
  });
});
