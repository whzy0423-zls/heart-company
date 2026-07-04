const PUSH_MEMBER_LEVELS = ['free', 'vip', 'svip'] as const;

export const pushMemberLevelOptions = [
  { label: '普通用户', value: 'free' },
  { label: 'VIP 会员', value: 'vip' },
  { label: '超级会员', value: 'svip' },
];

export function isValidPushMemberLevel(value: string) {
  return PUSH_MEMBER_LEVELS.includes(
    value as (typeof PUSH_MEMBER_LEVELS)[number],
  );
}

function stringValue(value: unknown) {
  return typeof value === 'string' ? value.trim() : '';
}

export function formatPushSendError(error: unknown) {
  const data =
    error && typeof error === 'object'
      ? (error as { message?: unknown; response?: { data?: Record<string, unknown> } })
          .response?.data
      : undefined;
  const detail =
    stringValue(data?.error) ||
    stringValue(data?.message) ||
    (error instanceof Error ? error.message.trim() : '');

  return detail ? `推送发送失败：${detail}` : '推送发送失败';
}

export function formatPushRecordError(record: {
  errorMessage?: string;
  status?: string;
}) {
  const detail = stringValue(record.errorMessage);
  return record.status === 'failed' && detail ? `失败原因：${detail}` : '';
}
