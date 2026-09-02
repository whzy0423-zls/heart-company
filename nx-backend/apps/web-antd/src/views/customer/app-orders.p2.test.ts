import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const source = readFileSync(resolve(__dirname, 'app-orders.vue'), 'utf8');

describe('App orders permission guards', () => {
  it('separates online reconciliation from legacy manual grants', () => {
    expect(source).toContain('Customer:AppOrders:Write');
    expect(source).toContain('useAccessStore');
    expect(source).toContain('canGrantOrder');
    expect(source).toMatch(
      /isLegacyManualOrder\(orderRecord\(record\)\).*record\.status !== 'paid'/s,
    );
    expect(source).toContain('isOnlineOrder');
    expect(source).toContain('isLegacyManualOrder');
    expect(source).toContain('主动查单');
    expect(source).toContain('确认开通');
    expect(source).toContain('reconcileAppOrderApi');
    expect(source).not.toMatch(/refundAppOrder|refundOrder|退款操作|退款按钮/);
  });

  it('shows every provider observation field', () => {
    for (const field of [
      'paymentProvider',
      'payChannel',
      'gatewayId',
      'providerStatus',
      'providerTradeNo',
      'lastQueryAt',
      'paymentError',
    ]) {
      expect(source).toContain(field);
    }
  });
});
