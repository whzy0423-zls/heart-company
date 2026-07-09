import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const assetsView = () =>
  readFileSync(resolve(root, 'views/video/assets.vue'), 'utf8');

const sectionBetween = (source: string, start: string, end: string) => {
  const startIndex = source.indexOf(start);
  const endIndex = source.indexOf(end, startIndex + start.length);

  expect(startIndex).toBeGreaterThanOrEqual(0);
  expect(endIndex).toBeGreaterThan(startIndex);

  return source.slice(startIndex, endIndex);
};

describe('video asset library OSS uploads', () => {
  it('stores public OSS objectUrl instead of local upload proxy url', () => {
    const source = assetsView();
    const upload = sectionBetween(
      source,
      'async function uploadAsset({ file }: UploadChangeParam) {',
      'function resetForm',
    );

    expect(source).toContain('requireUploadedPublicObjectUrl');
    expect(source).toContain("value.startsWith('http://') || value.startsWith('https://')");
    expect(upload).toContain('form.url = requireUploadedPublicObjectUrl(result,');
    expect(upload).not.toContain('form.url = result.objectUrl || result.url');
  });
});
