import { beforeEach, describe, expect, it, vi } from 'vitest';

import { requestClient } from '#/api/request';

import {
  getAppUserReportApi,
  getAppUserReportsApi,
  requestAppUserReportApi,
} from './app-user-report';

vi.mock('#/api/request', () => ({
  requestClient: { get: vi.fn(), post: vi.fn() },
}));

describe('growth analysis admin API', () => {
  beforeEach(() => vi.clearAllMocks());

  it('scopes report history to the selected user', async () => {
    await getAppUserReportsApi(12);
    expect(requestClient.get).toHaveBeenCalledWith('/app-user-reports', {
      params: { appUserId: 12 },
    });
  });

  it('loads a full report only by its selected id', async () => {
    await getAppUserReportApi(31);
    expect(requestClient.get).toHaveBeenCalledWith('/app-user-reports/31');
  });

  it('requests a background job without sending an override flag', async () => {
    await requestAppUserReportApi(12);
    expect(requestClient.post).toHaveBeenCalledWith('/app-user-reports', {
      appUserId: 12,
    });
  });
});
