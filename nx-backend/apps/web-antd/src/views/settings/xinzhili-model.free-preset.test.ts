import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const viewSource = readFileSync(resolve(__dirname, 'xinzhili-model.vue'), 'utf8');

describe('xinzhili free voice preset contract', () => {
  it('keeps realtime ASR on the Paraformer WebSocket contract', () => {
    expect(viewSource).toContain(
      "endpoint: 'wss://dashscope.aliyuncs.com/api-ws/v1/inference'",
    );
    expect(viewSource).toContain("model: 'paraformer-realtime-v2'");
    expect(viewSource).toContain("provider: 'aliyun-bailian'");
    expect(viewSource).toContain("region: 'cn-beijing'");
    expect(viewSource).not.toContain('SenseVoiceSmall');
  });

  it('offers a SiliconFlow TTS preset without replacing either API key', () => {
    expect(viewSource).toContain('填充免费额度 TTS 预设');
    expect(viewSource).toContain('function applyFreeTtsPreset()');
    expect(viewSource).toContain("endpoint: 'https://api.siliconflow.cn/v1'");
    expect(viewSource).toContain("model: 'FunAudioLLM/CosyVoice2-0.5B'");
    expect(viewSource).toContain(
      "voice: 'FunAudioLLM/CosyVoice2-0.5B:alex'",
    );
    expect(viewSource).toContain("format: 'mp3'");

    const presetBody = viewSource.match(
      /function applyFreeTtsPreset\(\) \{([\s\S]*?)\n\}/,
    )?.[1];
    expect(presetBody).toBeTruthy();
    expect(presetBody).not.toContain('realtimeAsr');
    expect(presetBody).not.toContain('apiKey');
  });

  it('explains the separate free-quota ASR and TTS setup', () => {
    expect(viewSource).toContain('实时 ASR 继续使用阿里云百炼 Paraformer');
    expect(viewSource).toContain('硅基流动免费额度');
    expect(viewSource).toContain('按钮不会覆盖已填写的 API Key');
  });
});
