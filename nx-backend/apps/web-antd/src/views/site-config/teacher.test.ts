import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

describe('teacher site-config editor terminology', () => {
  it('distinguishes teacher profile configuration from classroom media management', async () => {
    const source = await readFile(
      resolve('apps/web-antd/src/views/site-config/teacher.vue'),
      'utf8',
    );

    expect(source).toContain('老师资料管理');
    expect(source).toContain('老师简介');
    expect(source).toContain('课堂音视频内容');
    expect(source).toContain('老师课堂');
  });
});
