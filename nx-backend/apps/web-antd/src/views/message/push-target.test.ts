import { describe, expect, it, vi } from 'vitest';

import {
  audienceCountDetailLabel,
  audienceCountLabel,
  buildPushAudienceCountParams,
  formatPushRecordError,
  formatPushSendError,
  isValidPushMemberLevel,
  pushMemberLevelOptions,
  pushTemplates,
  refreshPushRecordsAfterSendAttempt,
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

  it('provides built-in push templates for common App scenarios', () => {
    expect(pushTemplates.length).toBeGreaterThanOrEqual(3);
    expect(pushTemplates.map((item) => item.key)).toContain('daily_practice');
    expect(pushTemplates[0]).toEqual(
      expect.objectContaining({
        content: expect.any(String),
        title: expect.any(String),
      }),
    );
  });

  it('builds audience count params only with valid target filters', () => {
    expect(
      buildPushAudienceCountParams({ targetType: 'all', targetValue: 'vip' }),
    ).toEqual({ targetType: 'all' });
    expect(
      buildPushAudienceCountParams({ targetType: 'level', targetValue: ' vip ' }),
    ).toEqual({ targetType: 'level', targetValue: 'vip' });
    expect(
      buildPushAudienceCountParams({ targetType: 'level', targetValue: 'gold' }),
    ).toEqual({ targetType: 'level' });
  });

  it('formats audience count labels', () => {
    expect(audienceCountLabel(0)).toBe('预计 0 人');
    expect(audienceCountLabel(128)).toBe('预计 128 人');
    expect(audienceCountLabel(undefined)).toBe('尚未预估');
    expect(audienceCountDetailLabel()).toBe('尚未预估');
    expect(audienceCountDetailLabel({ deviceCount: 14, userCount: 12 })).toBe(
      '预计 12 人 / 14 台设备',
    );
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

  it('refreshes push records after both successful and failed send attempts', async () => {
    const load = vi.fn().mockResolvedValue(undefined);

    await refreshPushRecordsAfterSendAttempt(load);

    expect(load).toHaveBeenCalledTimes(1);
  });

  it('does not mask send errors when refresh fails', async () => {
    const load = vi.fn().mockRejectedValue(new Error('list down'));
    const onError = vi.fn();

    await expect(
      refreshPushRecordsAfterSendAttempt(load, onError),
    ).resolves.toBeUndefined();

    expect(onError).toHaveBeenCalledWith('推送记录刷新失败，请手动刷新');
  });
});
