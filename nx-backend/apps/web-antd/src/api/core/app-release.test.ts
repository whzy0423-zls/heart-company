import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  upload: vi.fn(),
}));

vi.mock('#/api/request', () => ({
  requestClient: {
    get: vi.fn(),
    post: vi.fn(),
    request: mocks.request,
    upload: mocks.upload,
  },
}));

describe('app release api', () => {
  beforeEach(() => {
    mocks.request.mockReset();
    mocks.upload.mockReset();
  });

  it('updates a release policy with a complete PATCH body', async () => {
    const { updateAppReleasePolicyApi } = await import('./app-release');
    const policy = {
      forceUpdate: true,
      minSupportedVersionCode: 120,
      rolloutPercentage: 35,
    };

    await updateAppReleasePolicyApi(7, policy);

    expect(mocks.request).toHaveBeenCalledWith('/app-releases/7/policy', {
      data: policy,
      method: 'PATCH',
    });
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
