import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

describe('auth login slogan logo', () => {
  it('uses the existing brand logo image as the center graphic', () => {
    const svg = readFileSync(
      resolve(__dirname, '../../public/login-slogan.svg'),
      'utf8',
    );

    expect(svg).toContain('href="data:image/png;base64,');
    expect(svg).toContain('中央品牌 logo');
    expect(svg).not.toContain('href="/logo.png"');
    expect(svg).not.toContain('中央立体九宫格盘');
    expect(svg).not.toContain('行1：1 5 6 绿');
    expect(svg).not.toContain('rect x="181"');
  });
});
