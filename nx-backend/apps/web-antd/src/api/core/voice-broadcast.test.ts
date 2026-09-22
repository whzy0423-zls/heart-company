import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const here = dirname(fileURLToPath(import.meta.url));

describe('voice broadcast admin contract', () => {
  it('exposes versioned safe config APIs', () => {
    const source = readFileSync(resolve(here, 'voice-broadcast.ts'), 'utf8');
    for (const expected of [
      "'/app/voice-broadcast-config'",
      "'/app/voice-broadcast-config/test'",
      'expectedVersion',
      'apiKeySet',
      'apiKeySuffix',
    ]) {
      expect(source).toContain(expected);
    }
  });

  it('registers the page under App 管理', () => {
    const source = readFileSync(
      resolve(here, '../../router/routes/modules/app.ts'),
      'utf8',
    );
    expect(source).toContain("title: '语音播报配置'");
    expect(source).toContain("authority: ['App:VoiceBroadcast:Manage']");
    expect(source).toContain("path: 'voice-broadcast-config'");
  });

  it('keeps the key write-only and offers test/save actions', () => {
    const source = readFileSync(
      resolve(here, '../../views/app/voice-broadcast.vue'),
      'utf8',
    );
    for (const expected of [
      'Input.Password',
      '测试合成',
      '保存配置',
      'apiKeySet',
      'voice-broadcast-current-voice',
      'voice_broadcast_config_version_conflict',
    ]) {
      expect(source).toContain(expected);
    }
  });
});
