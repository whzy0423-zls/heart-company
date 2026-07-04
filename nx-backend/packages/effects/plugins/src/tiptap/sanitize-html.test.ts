import { describe, expect, it } from 'vitest';

import { sanitizeTipTapHTML } from './sanitize-html';

describe('sanitizeTipTapHTML', () => {
  it('removes scripts, event handlers, and unsafe URLs', () => {
    const html = sanitizeTipTapHTML(`
      <p onclick="alert(1)">hello <strong>world</strong></p>
      <script>alert(1)</script>
      <img src="javascript:alert(1)" onerror="alert(2)" alt="bad" />
      <a href="javascript:alert(3)" target="_blank">bad link</a>
      <svg onload="alert(4)"></svg>
    `);

    expect(html).toContain('<p>hello <strong>world</strong></p>');
    expect(html).not.toContain('script');
    expect(html).not.toContain('onclick');
    expect(html).not.toContain('onerror');
    expect(html).not.toContain('javascript:');
    expect(html).not.toContain('<svg');
    expect(html).not.toContain('href=');
    expect(html).not.toContain('src=');
  });

  it('keeps safe Tiptap formatting and safe media attributes', () => {
    const html = sanitizeTipTapHTML(`
      <h2 style="text-align:center;color:#123456">Title</h2>
      <blockquote><a href="/docs" target="_blank" rel="nofollow">Docs</a></blockquote>
      <img src="https://cdn.example.com/a.png" alt="cover" width="320" height="180" />
    `);

    expect(html).toContain('<h2 style="text-align: center; color: #123456;">Title</h2>');
    expect(html).toContain('<blockquote>');
    expect(html).toContain('<a href="/docs" target="_blank" rel="nofollow noopener noreferrer">Docs</a>');
    expect(html).toContain(
      '<img src="https://cdn.example.com/a.png" alt="cover" width="320" height="180">',
    );
  });
});
