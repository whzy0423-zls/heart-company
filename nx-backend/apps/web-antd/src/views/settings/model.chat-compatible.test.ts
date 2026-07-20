import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const viewSource = readFileSync(resolve(__dirname, 'model.vue'), 'utf8');
const apiSource = readFileSync(
  resolve(__dirname, '../../api/core/model-config.ts'),
  'utf8',
);
const chatSection = viewSource.slice(
  viewSource.indexOf('对话模型（手机端聊天窗口作答所用）'),
  viewSource.indexOf('视频模型'),
);

describe('chat model compatible protocol form', () => {
  it('offers OpenAI and Anthropic compatible protocols', () => {
    expect(viewSource).toContain("value: 'openai-compatible'");
    expect(viewSource).toContain("value: 'anthropic-compatible'");
    expect(chatSection).toContain('v-model:value="form.chat.provider"');
  });

  it('uses coding-play as the default base and removes MiniMax Group ID', () => {
    expect(viewSource).toContain("apiBase: 'https://coding-play.codes'");
    expect(chatSection).not.toContain('Group ID');
    expect(chatSection).not.toContain('abab6.5s-chat');
  });

  it('submits provider without a chat groupId field', () => {
    const chatType = apiSource.slice(
      apiSource.indexOf('chat: {'),
      apiSource.indexOf('video: {'),
    );
    expect(chatType).toContain('provider: string');
    expect(chatType).not.toContain('groupId');
  });
});
