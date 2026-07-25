import { describe, expect, it } from 'vitest';

import { bookingSignupTarget, miniappOpenIntent } from './miniapp-user';

describe('miniapp customer route helpers', () => {
  it('accepts only positive integer user and record ids', () => {
	expect(miniappOpenIntent({ userId: '7', open: 'detail' })).toEqual({ mode: 'detail', userId: '7' });
	expect(miniappOpenIntent({ userId: '7', testRecordId: '11', open: 'test' })).toEqual({ mode: 'test', testRecordId: '11', userId: '7' });
	expect(miniappOpenIntent({ userId: '../7', open: 'detail' })).toBeUndefined();
  });

  it('builds a fixed signup route without accepting arbitrary paths', () => {
	expect(bookingSignupTarget('91')).toEqual({ path: '/customer/signups', query: { leadId: '91', open: 'detail' } });
	expect(bookingSignupTarget('javascript:alert(1)')).toBeUndefined();
  });
});
