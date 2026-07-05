import { afterEach, describe, expect, it, vi } from 'vitest';
import { effectScope } from 'vue';

import {
  createUploadAssetObjectURL,
  isProtectedUploadAssetSource,
  useUploadAssetPreviewResolver,
} from './upload-asset-preview';

const originalFetch = globalThis.fetch;
const originalCreateObjectURL = URL.createObjectURL;
const originalRevokeObjectURL = URL.revokeObjectURL;

describe('upload asset preview urls', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    globalThis.fetch = originalFetch;
    URL.createObjectURL = originalCreateObjectURL;
    URL.revokeObjectURL = originalRevokeObjectURL;
  });

  it('fetches protected upload assets with Authorization and returns a blob url', async () => {
    const blob = new Blob(['image'], { type: 'image/png' });
    const fetchMock = vi.fn().mockResolvedValue({
      blob: () => Promise.resolve(blob),
      ok: true,
    });
    globalThis.fetch = fetchMock;
    URL.createObjectURL = vi.fn().mockReturnValue('blob:preview');

    const url = await createUploadAssetObjectURL('/api/upload-assets/1', 'abc');

    expect(url).toBe('blob:preview');
    expect(fetchMock).toHaveBeenCalledWith('/api/upload-assets/1', {
      headers: {
        Authorization: 'Bearer abc',
      },
    });
  });

  it('leaves public urls unchanged', async () => {
    const fetchMock = vi.fn();
    globalThis.fetch = fetchMock;

    const url = await createUploadAssetObjectURL(
      'https://cdn.example.com/a.png',
      'abc',
    );

    expect(url).toBe('https://cdn.example.com/a.png');
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('rejects unsafe non-protected preview urls', async () => {
    const fetchMock = vi.fn();
    globalThis.fetch = fetchMock;

    await expect(
      createUploadAssetObjectURL('javascript:alert(1)', 'abc'),
    ).resolves.toBe('');
    await expect(
      createUploadAssetObjectURL(
        'data:text/html,<script>alert(1)</script>',
        'abc',
      ),
    ).resolves.toBe('');
    await expect(
      createUploadAssetObjectURL('//evil.example/a.png', 'abc'),
    ).resolves.toBe('');
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('recognizes same-origin absolute upload asset urls as protected', () => {
    expect(isProtectedUploadAssetSource('/api/upload-assets/1')).toBe(true);
    expect(
      isProtectedUploadAssetSource(
        `${window.location.origin}/api/upload-assets/1`,
      ),
    ).toBe(true);
    expect(
      isProtectedUploadAssetSource('https://cdn.example.com/api/upload-assets/1'),
    ).toBe(false);
  });

  it('recognizes legacy same-origin uploads urls as protected', () => {
    expect(isProtectedUploadAssetSource('/api/uploads/site/logo.png')).toBe(
      true,
    );
    expect(
      isProtectedUploadAssetSource(
        `${window.location.origin}/api/uploads/site/logo.png`,
      ),
    ).toBe(true);
    expect(
      isProtectedUploadAssetSource('https://cdn.example.com/api/uploads/logo.png'),
    ).toBe(false);
  });

  it('revokes the oldest cached blob preview when the resolver cache is full', async () => {
    const fetchMock = vi.fn().mockImplementation(() =>
      Promise.resolve({
        blob: () => Promise.resolve(new Blob(['image'], { type: 'image/png' })),
        ok: true,
      }),
    );
    globalThis.fetch = fetchMock;
    let objectURLIndex = 0;
    URL.createObjectURL = vi
      .fn()
      .mockImplementation(() => `blob:preview-${++objectURLIndex}`);
    URL.revokeObjectURL = vi.fn();

    const scope = effectScope();
    const resolver = scope.run(() => useUploadAssetPreviewResolver(() => 'abc'));
    if (!resolver) {
      throw new Error('resolver was not created');
    }

    for (let index = 1; index <= 25; index++) {
      expect(resolver.resolve(`/api/upload-assets/${index}`)).toBe('');
    }
    await new Promise((resolve) => window.setTimeout(resolve, 0));

    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:preview-1');
    expect(resolver.resolve('/api/upload-assets/1')).toBe('');
    expect(fetchMock).toHaveBeenCalledTimes(26);
    scope.stop();
  });

  it('limits concurrent resolver preview fetches', async () => {
    const responses: Array<(value: unknown) => void> = [];
    const fetchMock = vi.fn().mockImplementation(
      () =>
        new Promise((resolve) => {
          responses.push(resolve);
        }),
    );
    globalThis.fetch = fetchMock;
    URL.createObjectURL = vi.fn().mockReturnValue('blob:preview');

    const scope = effectScope();
    const resolver = scope.run(() => useUploadAssetPreviewResolver(() => 'abc'));
    if (!resolver) {
      throw new Error('resolver was not created');
    }

    for (let index = 1; index <= 10; index++) {
      resolver.resolve(`/api/upload-assets/${index}`);
    }

    expect(fetchMock).toHaveBeenCalledTimes(4);
    responses[0]?.({
      blob: () => Promise.resolve(new Blob(['image'], { type: 'image/png' })),
      ok: true,
    });
    await new Promise((resolve) => window.setTimeout(resolve, 0));

    expect(fetchMock).toHaveBeenCalledTimes(5);
    scope.stop();
  });

  it('aborts active resolver preview fetches when revoked', () => {
    let signal: AbortSignal | undefined;
    const fetchMock = vi.fn().mockImplementation((_url, init) => {
      signal = init?.signal;
      return new Promise(() => {});
    });
    globalThis.fetch = fetchMock;

    const scope = effectScope();
    const resolver = scope.run(() => useUploadAssetPreviewResolver(() => 'abc'));
    if (!resolver) {
      throw new Error('resolver was not created');
    }

    resolver.resolve('/api/upload-assets/1');
    expect(signal?.aborted).toBe(false);

    resolver.revokeAll();

    expect(signal?.aborted).toBe(true);
    scope.stop();
  });
});
