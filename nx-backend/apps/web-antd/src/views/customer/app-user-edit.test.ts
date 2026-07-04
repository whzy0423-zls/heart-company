import type { AppCustomer } from '#/api';

import { describe, expect, it, vi } from 'vitest';

import { requestClient } from '#/api/request';
import { updateAppCustomerApi } from '#/api/core/app-customer';

import {
  buildAppCustomerUpdatePayload,
  canEditAppCustomer,
  createAppCustomerEditForm,
} from './app-user-edit';

vi.mock('#/api/request', () => ({
  requestClient: {
    put: vi.fn(),
  },
}));

function customer(input?: Partial<AppCustomer>): AppCustomer {
  return {
    avatar: '',
    createTime: '2026/01/01 10:00:00',
    id: 42,
    lastLoginAt: null,
    memberLevel: 'free',
    nickname: '测试客户',
    phone: '13800000021',
    registerSource: 'app_sms',
    status: 'active',
    updateTime: '2026/01/01 10:00:00',
    ...input,
  };
}

describe('app customer edit helpers', () => {
  it('creates an editable form from the selected customer', () => {
    expect(createAppCustomerEditForm(customer())).toEqual({
      memberLevel: 'free',
      status: 'active',
    });
  });

  it('builds a minimal update payload for status and member level', () => {
    expect(
      buildAppCustomerUpdatePayload({
        memberLevel: 'vip',
        status: 'disabled',
      }),
    ).toEqual({
      memberLevel: 'vip',
      status: 'disabled',
    });
  });

  it('requires the App customer write permission for edit actions', () => {
    expect(canEditAppCustomer(['Customer:App:List'])).toBe(false);
    expect(canEditAppCustomer(['Customer:App:List', 'Customer:App:Write'])).toBe(
      true,
    );
  });
});

describe('updateAppCustomerApi', () => {
  it('updates status and member level with PUT', async () => {
    const put = vi.mocked(requestClient.put);
    put.mockResolvedValue(customer({ memberLevel: 'vip', status: 'disabled' }));

    await updateAppCustomerApi(42, {
      memberLevel: 'vip',
      status: 'disabled',
    });

    expect(put).toHaveBeenCalledWith('/app-users/42', {
      memberLevel: 'vip',
      status: 'disabled',
    });
  });
});
