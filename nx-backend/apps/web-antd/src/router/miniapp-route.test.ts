import { describe, expect, it } from 'vitest';

import routes from './routes/modules/miniapp';

describe('miniapp management routes', () => {
  it('exposes customer information under miniapp management', () => {
    const parent = routes.find((route) => route.name === 'MiniappManage');
    const customer = parent?.children?.find(
      (route) => route.name === 'MiniappCustomers',
    );

    expect(parent?.meta?.authority).toEqual([
      'Website:Write',
      'Customer:Miniapp:List',
    ]);
    expect(customer?.path).toBe('customers');
    expect(customer?.meta?.title).toBe('客户信息');
    expect(customer?.meta?.authority).toEqual(['Customer:Miniapp:List']);
  });
});
