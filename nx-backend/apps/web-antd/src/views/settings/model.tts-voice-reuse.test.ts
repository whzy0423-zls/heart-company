import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const viewSource = readFileSync(resolve(__dirname, 'xinzhili-model.vue'), 'utf8');
const apiSource = readFileSync(
  resolve(__dirname, '../../api/core/xinzhili-model-config.ts'),
  'utf8',
);
const voiceApiSource = readFileSync(
  resolve(__dirname, '../../api/core/voice.ts'),
  'utf8',
);

describe('xinzhili TTS voice reuse configuration', () => {
  it('adds a TTS config payload/view with a persisted voice id', () => {
    expect(apiSource).toContain('tts: {');
    const ttsApiSource = apiSource.slice(
      apiSource.indexOf('tts: {'),
      apiSource.indexOf('};', apiSource.indexOf('tts: {')),
    );
    expect(ttsApiSource).toContain("provider: 'bailian' | 'minimax' | 'openai-compatible'");
    expect(ttsApiSource).toContain('endpoint: string');
    expect(ttsApiSource).toContain('groupId?: string');
    expect(ttsApiSource).toContain('model: string');
    expect(ttsApiSource).toContain('voice: string');
    expect(ttsApiSource).toContain("format: 'mp3'");
  });

  it('reuses voice management options on MiniMax TTS and keeps manual voice fallback', () => {
    expect(viewSource).toContain('AI 语音合成 / TTS 配置');
    expect(viewSource).toContain('getVoiceOptionsApi');
    expect(viewSource).toContain('groupedTtsVoiceOptions');
    expect(viewSource).toContain('选择已有音色');
    expect(viewSource).toContain('可直接复用声音管理里的已克隆音色');
    expect(viewSource).toContain('音色选项读取失败，可手动填写音色 ID');
    expect(viewSource).toContain('v-if="canSelectExistingTtsVoice"');
    expect(viewSource).toContain('v-model:value="form.tts.voice"');
    expect(viewSource).toContain('@change="handleTtsVoiceOptionChange"');
  });

  it('supports Aliyun Bailian Bailian TTS with same-provider cloned voices', () => {
    expect(viewSource).toContain("{ label: '阿里百炼', value: 'bailian' }");
    expect(viewSource).toContain('filteredTtsVoiceOptions');
    expect(viewSource).toContain('voiceOptionProvider(item) === currentTtsVoiceProvider.value');
    expect(viewSource).toContain('applyTtsProviderPreset');
    expect(viewSource).toContain('https://dashscope.aliyuncs.com/api/v1');
    expect(viewSource).toContain('MiniMax/speech-2.8-turbo');
    expect(viewSource).toContain("if (provider === 'bailian')");
  });

  it('normalizes legacy TTS OpenAI-compatible provider to Bailian before display and save', () => {
    expect(viewSource).toContain('function normalizeTtsProvider');
    expect(viewSource).toContain("normalizedEndpoint.includes('dashscope.aliyuncs.com/compatible-mode')");
    expect(viewSource).toContain('const provider = normalizeTtsProvider(');
    expect(viewSource).toContain('legacyAliyunBailianTtsPreset');
  });

  it('documents voice options as clone or official sources', () => {
    expect(voiceApiSource).toContain("source: 'clone' | 'official'");
    expect(voiceApiSource).toContain('provider: string');
    expect(voiceApiSource).toContain('voiceId: string');
    expect(voiceApiSource).toContain('voiceName: string');
  });
});
