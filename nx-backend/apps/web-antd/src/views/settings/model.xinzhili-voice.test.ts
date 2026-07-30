import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const viewSource = readFileSync(resolve(__dirname, 'model.vue'), 'utf8');
const xinzhiliModelSource = readFileSync(
  resolve(__dirname, 'xinzhili-model.vue'),
  'utf8',
);
const apiSource = readFileSync(
  resolve(__dirname, '../../api/core/model-config.ts'),
  'utf8',
);
const section = viewSource.slice(
  viewSource.indexOf('芯之力语音配置'),
  viewSource.indexOf('智能辅助作答'),
);
const siliconFlowPreset = viewSource.slice(
  viewSource.indexOf('function applySiliconFlowVoicePreset'),
  viewSource.indexOf('async function save'),
);

describe('xinzhili voice model form', () => {
  it('专用芯之力页不再提供硅基流动语音预设', () => {
    expect(xinzhiliModelSource).not.toContain('api.siliconflow.cn');
    expect(xinzhiliModelSource).not.toContain('applyFreeTtsPreset');
    expect(xinzhiliModelSource).not.toContain('填充免费额度 TTS 预设');
  });

  it('只声明一次对话协议选项，避免模型配置页构建失败', () => {
    expect(viewSource.match(/const chatProviderOptions\s*=/g)).toHaveLength(1);
    expect(
      viewSource.match(/<section data-testid="chat-model-section">/g),
    ).toHaveLength(1);
    expect(viewSource.match(/<\/section>/g)).toHaveLength(1);
  });

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

  it('一键填充硅基流动免费 ASR 与 TTS 预设且保留已输入密钥', () => {
    expect(section).toContain('@click="applySiliconFlowVoicePreset"');
    expect(section).toContain('使用硅基流动免费预设');

    expect(siliconFlowPreset).toContain("provider: 'openai-compatible'");
    expect(siliconFlowPreset).toContain(
      "apiBase: 'https://api.siliconflow.cn/v1'",
    );
    expect(siliconFlowPreset).toContain("model: 'FunAudioLLM/SenseVoiceSmall'");
    expect(siliconFlowPreset).toContain("language: 'zh'");
    expect(siliconFlowPreset).toContain("model: 'FunAudioLLM/CosyVoice2-0.5B'");
    expect(siliconFlowPreset).toContain(
      "voice: 'FunAudioLLM/CosyVoice2-0.5B:alex'",
    );
    expect(siliconFlowPreset).toContain("responseFormat: 'mp3'");
    expect(siliconFlowPreset).toContain('speed: 1');
    expect(siliconFlowPreset).toContain('timeoutSeconds: 30');
    expect(siliconFlowPreset).toContain('timeoutSeconds: 45');
    expect(siliconFlowPreset).not.toMatch(/apiKey\s*:/);
    expect(siliconFlowPreset).not.toMatch(/\.apiKey\s*=/);
  });
});
