import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  upload: vi.fn(),
}));

vi.mock('#/api/request', () => ({
  requestClient: {
    get: vi.fn(),
    post: vi.fn(),
    upload: mocks.upload,
  },
}));

describe('app release api', () => {
  beforeEach(() => {
    mocks.upload.mockReset();
  });

  it('allows large APK uploads to run for the proxy upload window', async () => {
    const { uploadAppReleaseApi } = await import('./app-release');
    const file = new File(['apk'], 'nine-xing.apk', {
      type: 'application/vnd.android.package-archive',
    });
    const onUploadProgress = vi.fn();

    await uploadAppReleaseApi(file, 'release notes', onUploadProgress);

    expect(mocks.upload).toHaveBeenCalledWith(
      '/app-releases/upload',
      { file, release_notes: 'release notes' },
      {
        onUploadProgress,
        timeout: 1_800_000,
      },
    );
  });
});
