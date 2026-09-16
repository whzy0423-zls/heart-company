import type { Dayjs } from 'dayjs';
import type { GrantAppTrialCreditsInput } from '#/api';

import dayjs from 'dayjs';

export interface AppTrialCreditForm {
  amount: number;
  expiresAt: string;
  reason: string;
}

export function createAppTrialCreditForm(now: Dayjs = dayjs()): AppTrialCreditForm {
  return {
    amount: 10,
    expiresAt: now.add(3, 'day').format('YYYY-MM-DDTHH:mm'),
    reason: '',
  };
}

export function validateAppTrialCreditForm(
  form: AppTrialCreditForm,
  now: Dayjs = dayjs(),
): string {
  if (!Number.isInteger(form.amount) || form.amount < 1 || form.amount > 1000) {
    return '赠送次数必须在 1 到 1000 之间';
  }
  const reason = form.reason.trim();
  if (!reason || [...reason].length > 200) {
    return '赠送原因必填且不能超过 200 字';
  }
  const expiresAt = dayjs(form.expiresAt);
  if (!expiresAt.isValid() || !expiresAt.isAfter(now)) {
    return '到期时间必须晚于当前时间';
  }
  return '';
}

export function buildAppTrialCreditPayload(
  form: AppTrialCreditForm,
  idempotencyKey: string,
): GrantAppTrialCreditsInput {
  return {
    amount: form.amount,
    expiresAt: dayjs(form.expiresAt).toISOString(),
    idempotencyKey,
    reason: form.reason.trim(),
  };
}

export function createAppTrialCreditIdempotencyKey(userId: number | string) {
  const random = globalThis.crypto?.randomUUID?.() ??
    `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `trial-credit-${userId}-${random}`;
}

export function appTrialCreditStatusLabel(status: string) {
  return {
    active: '有效',
    exhausted: '已用完',
    expired: '已过期',
    revoked: '已撤销',
  }[status] ?? status;
}
