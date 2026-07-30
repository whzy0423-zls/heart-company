import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const viewSource = readFileSync(
  resolve(__dirname, 'xinzhili-model.vue'),
  'utf8',
);

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

  it('removes the SiliconFlow free TTS preset from this page', () => {
    expect(viewSource).not.toContain('api.siliconflow.cn');
    expect(viewSource).not.toContain('applyFreeTtsPreset');
    expect(viewSource).not.toContain('填充免费额度 TTS 预设');
    expect(viewSource).not.toContain('硅基流动免费额度');
    expect(viewSource).not.toContain('免费额度配置说明');
  });

  it('documents one Bailian key for the full realtime voice chain', () => {
    expect(viewSource).toContain('BailianCredentialsCard');
    expect(viewSource).toContain('Paraformer');
    expect(viewSource).toContain('Qwen 克隆音色');
    expect(viewSource).toContain('Qwen TTS');
  });
});
