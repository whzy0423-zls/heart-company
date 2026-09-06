import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const view = readFileSync(resolve(__dirname, 'mode.vue'), 'utf8');
const routes = readFileSync(
  resolve(__dirname, '../../router/routes/modules/payment.ts'),
  'utf8',
);

describe('App payment mode management', () => {
  it('loads and saves the dedicated payment mode endpoint', () => {
    expect(view).toContain("'/admin/app-payment-mode'");
    expect(view).toContain("mode: 'customer_service'");
    expect(view).toContain('value="xzn"');
  });

  it('requires confirmation and exposes configuration readiness', () => {
    expect(view).toContain('Modal.confirm');
    expect(view).toContain('customerServiceConfigured');
    expect(view).toContain('xznConfigured');
    expect(view).toContain('仅影响新订单');
  });

  it('registers the independent payment mode route', () => {
    expect(routes).toContain("import('#/views/third-party-payment/mode.vue')");
    expect(routes).toContain("name: 'AppPaymentMode'");
    expect(routes).toContain("path: 'mode'");
  });
});
