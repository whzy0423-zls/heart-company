import { describe, expect, it } from 'vitest';

import { isSafePreviewURL } from './safe-preview-url';

describe('safe preview url', () => {
  it('allows normal web, blob, and relative preview urls', () => {
    expect(isSafePreviewURL('https://cdn.example.com/file.pdf')).toBe(true);
    expect(isSafePreviewURL('http://localhost:5320/file.pdf')).toBe(true);
    expect(isSafePreviewURL('blob:http://localhost/preview')).toBe(true);
    expect(isSafePreviewURL('/api/files/file.pdf')).toBe(true);
  });

  it('rejects executable or ambiguous preview urls', () => {
    expect(isSafePreviewURL('javascript:alert(1)')).toBe(false);
    expect(isSafePreviewURL('data:text/html,<script>alert(1)</script>')).toBe(
      false,
    );
    expect(isSafePreviewURL('//evil.example/file.pdf')).toBe(false);
    expect(isSafePreviewURL('file:///etc/passwd')).toBe(false);
    expect(isSafePreviewURL('http://cdn.example.com/file.pdf')).toBe(false);
  });
});
