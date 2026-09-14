import dayjs from 'dayjs';
import { describe, expect, it } from 'vitest';

import {
  buildAppTrialCreditPayload,
  createAppTrialCreditForm,
  validateAppTrialCreditForm,
} from './app-trial-credit';

describe('App trial credit form', () => {
  it('defaults expiry to three days after opening', () => {
    const now = dayjs('2026-09-14T10:00:00+08:00');
    const form = createAppTrialCreditForm(now);
    expect(dayjs(form.expiresAt).diff(now, 'hour')).toBe(72);
  });

  it('validates amount, expiry, and reason', () => {
    const now = dayjs('2026-09-14T10:00:00+08:00');
    expect(
      validateAppTrialCreditForm(
        { amount: 0, expiresAt: now.add(3, 'day').format(), reason: '推广' },
        now,
      ),
    ).toContain('1 到 1000');
    expect(
      validateAppTrialCreditForm(
        { amount: 10, expiresAt: now.subtract(1, 'minute').format(), reason: '推广' },
        now,
      ),
    ).toContain('晚于当前时间');
    expect(
      validateAppTrialCreditForm(
        { amount: 10, expiresAt: now.add(3, 'day').format(), reason: ' ' },
        now,
      ),
    ).toContain('原因');
  });

  it('builds an RFC3339 payload with the supplied idempotency key', () => {
    const payload = buildAppTrialCreditPayload(
      {
        amount: 20,
        expiresAt: '2026-09-17T10:00',
        reason: ' 分享活动奖励 ',
      },
      'grant-key',
    );
    expect(payload.amount).toBe(20);
    expect(payload.reason).toBe('分享活动奖励');
    expect(payload.idempotencyKey).toBe('grant-key');
    expect(dayjs(payload.expiresAt).isValid()).toBe(true);
  });
});
