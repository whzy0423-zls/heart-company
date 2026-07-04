import { describe, expect, it } from 'vitest';

import { safeIframeSrc } from './iframe-src';

describe('safeIframeSrc', () => {
  const base = 'https://admin.example.com';

  it('allows same-origin relative and absolute URLs', () => {
    expect(safeIframeSrc('/reports/overview', base)).toBe(
      'https://admin.example.com/reports/overview',
    );
    expect(safeIframeSrc('https://admin.example.com/docs', base)).toBe(
      'https://admin.example.com/docs',
    );
  });

  it('rejects unsafe iframe sources', () => {
    for (const source of [
      '',
      ' javascript:alert(1)',
      'data:text/html,<script>alert(1)</script>',
      'blob:https://admin.example.com/id',
      '//evil.example.com/frame',
      'https://evil.example.com/frame',
      'https://admin.example.com\u0000.evil.example/frame',
    ]) {
      expect(safeIframeSrc(source, base)).toBe('about:blank');
    }
  });
});
