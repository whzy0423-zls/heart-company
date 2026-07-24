import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const viewSource = readFileSync(resolve(__dirname, 'xinzhili-model.vue'), 'utf8');
const apiSource = readFileSync(
  resolve(__dirname, '../../api/core/xinzhili-model-config.ts'),
  'utf8',
);
const apiIndexSource = readFileSync(
  resolve(__dirname, '../../api/core/index.ts'),
  'utf8',
);

describe('xinzhili model configuration page contract', () => {
  it('uses the dedicated versioned API and preserves blank secrets', () => {
    expect(apiSource).toContain("'/xinzhili-model-config'");
    expect(apiSource).toContain('expectedVersion: number');
    expect(apiSource).toContain('apiKeySet: boolean');
    expect(apiIndexSource).toContain("export * from './xinzhili-model-config';");
    expect(viewSource).toContain('留空表示不修改');
    expect(viewSource).toContain('saved.version');
  });

  it('keeps normal enabled and handles version conflicts explicitly', () => {
    expect(viewSource).toContain("value: 'normal'");
    expect(viewSource).toContain(':disabled="mode.value === \'normal\'"');
    expect(viewSource).toContain('status === 409');
    expect(viewSource).toContain('配置已被其他管理员修改，请重新加载');
  });

  it('supports both TTS providers and exposes timing and prompts', () => {
    expect(viewSource).toContain("value: 'openai-compatible'");
    expect(viewSource).toContain("value: 'minimax'");
    expect(viewSource).toContain('form.timing.partialStableMs');
    expect(viewSource).toContain('form.commonPrompt');
    expect(viewSource).toContain('form.modePrompts[mode.value]');
  });
});
