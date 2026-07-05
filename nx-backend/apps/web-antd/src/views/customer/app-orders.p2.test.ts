import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const source = readFileSync(resolve(__dirname, 'app-orders.vue'), 'utf8');

describe('App orders permission guards', () => {
  it('shows grant action only to Customer:AppOrders:Write users', () => {
    expect(source).toContain("Customer:AppOrders:Write");
    expect(source).toContain('useAccessStore');
    expect(source).toContain('canGrantOrder');
    expect(source).toMatch(/v-if="record\.status !== 'paid' && canGrantOrder"/);
  });
});
