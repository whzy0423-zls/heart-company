import { describe, expect, it } from 'vitest';

import {
  discountedPriceCents,
  validateDiscountRule,
} from './agent-discount-pricing';

describe('agent discount pricing preview', () => {
  const base = {
    audience: 'agent_self' as const,
    enabled: true,
    productId: 'svip_month',
  };

  it('deducts basis-point percentage and rounds the reduction to cents', () => {
    expect(
      discountedPriceCents(5900, {
        ...base,
        mode: 'percent_off',
        value: 1250,
      }),
    ).toBe(5162);
  });

  it('deducts a fixed amount in cents', () => {
    expect(
      discountedPriceCents(5900, {
        ...base,
        mode: 'amount_off',
        value: 500,
      }),
    ).toBe(5400);
  });

  it('keeps a one-cent payable amount when percentage rounding reaches the full price', () => {
    const rule = { ...base, mode: 'percent_off' as const, value: 9999 };
    expect(discountedPriceCents(50, rule)).toBe(1);
    expect(validateDiscountRule(50, rule)).toBeNull();
  });

  it('uses the base price when the rule is disabled', () => {
    expect(
      discountedPriceCents(5900, {
        ...base,
        enabled: false,
        mode: 'percent_off',
        value: 2500,
      }),
    ).toBe(5900);
  });

  it('rejects discounts that would make a paid plan free', () => {
    expect(
      validateDiscountRule(5900, {
        ...base,
        mode: 'percent_off',
        value: 10_000,
      }),
    ).not.toBeNull();
    expect(
      validateDiscountRule(5900, {
        ...base,
        mode: 'amount_off',
        value: 5900,
      }),
    ).not.toBeNull();
  });

  it('accepts disabled rules without a discount value', () => {
    expect(
      validateDiscountRule(5900, {
        ...base,
        enabled: false,
        mode: 'percent_off',
        value: 0,
      }),
    ).toBeNull();
  });
});
