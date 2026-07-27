import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

describe('course site-config editor terminology', () => {
  it('labels legacy course cards as course products and separates classroom media', async () => {
    const source = await readFile(
      resolve('apps/web-antd/src/views/site-config/courses.vue'),
      'utf8',
    );

    expect(source).toContain('课程产品管理');
    expect(source).toContain('课程产品卡片');
    expect(source).toContain('课堂音视频内容');
    expect(source).toContain('老师课堂');
  });
});
