import { describe, expect, it } from 'vitest';

import {
  buildMembershipGrantPayload,
  memberPlanLabel,
  membershipStatusLabel,
  previewMembershipExpiry,
} from './app-membership';

describe('app membership helpers', () => {
  it('labels exact membership plans', () => {
    expect(memberPlanLabel('vip_month')).toBe('月包会员');
    expect(memberPlanLabel('vip_quarter')).toBe('季包会员');
    expect(memberPlanLabel('vip_year')).toBe('年包会员');
    expect(memberPlanLabel('free')).toBe('普通用户');
  });

  it('labels pending customer confirmation separately from paid orders', () => {
    expect(membershipStatusLabel('pending_confirmation')).toBe('待客服确认');
    expect(membershipStatusLabel('paid')).toBe('已开通');
  });

  it('builds an RFC3339 activation payload', () => {
    expect(
      buildMembershipGrantPayload(new Date('2026-07-20T10:30:00+08:00')),
    ).toEqual({ activationAt: '2026-07-20T02:30:00.000Z' });
  });

  it('previews renewal after the current active expiry', () => {
    expect(
      previewMembershipExpiry(
        '2026-08-01T00:00:00.000Z',
        new Date('2026-07-20T00:00:00.000Z'),
        30,
      )?.toISOString(),
    ).toBe('2026-08-31T00:00:00.000Z');
  });

  it('previews renewal from activation when membership has expired', () => {
    expect(
      previewMembershipExpiry(
        '2026-07-01T00:00:00.000Z',
        new Date('2026-07-20T00:00:00.000Z'),
        90,
      )?.toISOString(),
    ).toBe('2026-10-18T00:00:00.000Z');
  });
});
