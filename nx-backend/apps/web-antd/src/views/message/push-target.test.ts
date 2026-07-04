import { describe, expect, it } from 'vitest';

import {
  formatPushRecordError,
  formatPushSendError,
  isValidPushMemberLevel,
  pushMemberLevelOptions,
} from './push-target';

describe('push target helpers', () => {
  it('only accepts supported App member levels', () => {
    expect(isValidPushMemberLevel('free')).toBe(true);
    expect(isValidPushMemberLevel('vip')).toBe(true);
    expect(isValidPushMemberLevel('svip')).toBe(true);
    expect(isValidPushMemberLevel('gold')).toBe(false);
    expect(isValidPushMemberLevel('VIP')).toBe(false);
  });

  it('exposes concrete member level select options', () => {
    expect(pushMemberLevelOptions.map((item) => item.value)).toEqual([
      'free',
      'vip',
      'svip',
    ]);
  });

  it('uses backend error details for failed push send messages', () => {
    expect(
      formatPushSendError({
        response: { data: { error: 'push sender is not configured' } },
      }),
    ).toBe('推送发送失败：push sender is not configured');
    expect(
      formatPushSendError({
        response: { data: { message: 'JPush 认证失败' } },
      }),
    ).toBe('推送发送失败：JPush 认证失败');
    expect(formatPushSendError(new Error('network down'))).toBe(
      '推送发送失败：network down',
    );
  });

  it('formats failed push history error messages', () => {
    expect(
      formatPushRecordError({
        errorMessage: 'push sender is not configured',
        status: 'failed',
      }),
    ).toBe('失败原因：push sender is not configured');
    expect(formatPushRecordError({ errorMessage: '', status: 'success' })).toBe(
      '',
    );
  });
});
