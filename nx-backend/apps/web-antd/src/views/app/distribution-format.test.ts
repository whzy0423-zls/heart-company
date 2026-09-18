import { describe, expect, it } from 'vitest';
import { formatYuan, settlementActionPayload } from './distribution-format';

describe('distribution monetary and settlement contracts', () => {
  it('converts integer API cents to yuan', () => {
    expect(formatYuan(386800)).toBe('¥3,868');
    expect(formatYuan(10005)).toBe('¥100.05');
    expect(formatYuan(1)).toBe('¥0.01');
    expect(formatYuan(0)).toBe('¥0');
  });
  it('requires evidence for paid and reasons for cancellation or rejection', () => {
    expect(() => settlementActionPayload('paid', '')).toThrow();
    expect(() => settlementActionPayload('reject', ' ')).toThrow();
    expect(() => settlementActionPayload('cancel', '')).toThrow();
    expect(settlementActionPayload('paid', ' receipt-123 ')).toEqual({ paymentReference: 'receipt-123' });
    expect(settlementActionPayload('cancel', 'refund')).toEqual({ reason: 'refund' });
    expect(settlementActionPayload('approve', '')).toEqual({});
  });
});
