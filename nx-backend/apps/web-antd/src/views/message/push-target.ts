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

export function formatPushSendAcceptedMessage(result?: {
  message?: string;
  recordId?: number;
  status?: string;
}) {
  const message = stringValue(result?.message) || '推送任务已创建，后台发送中';
  const recordId =
    typeof result?.recordId === 'number' && Number.isFinite(result.recordId)
      ? result.recordId
      : 0;
  return recordId > 0 ? `${message}（记录 #${recordId}）` : message;
}

export async function refreshPushRecordsAfterSendAttempt(
  load: () => Promise<unknown>,
  onRefreshFailed?: (message: string) => void,
) {
  try {
    await load();
  } catch {
    onRefreshFailed?.('推送记录刷新失败，请手动刷新');
  }
}

export interface PushTemplate {
  content: string;
  deepLink?: string;
  key: string;
  targetType?: string;
  targetValue?: string;
  title: string;
}

export interface PushAudienceCountParams {
  targetType: string;
  targetValue?: string;
}

export const pushTemplates: PushTemplate[] = [
  {
    content: '今天 5 道题等你完成，花 1 分钟让系统更懂你。',
    deepLink: '/daily-quiz',
    key: 'daily_quiz',
    title: '今日画像校准题已准备好',
  },
  {
    content: '今日练习已更新，花 3 分钟记录一次真实的自己。',
    deepLink: '/daily',
    key: 'daily_practice',
    title: '今日练习提醒',
  },
  {
    content: '你的成长任务有新进展，继续完成可获得更完整的自我洞察。',
    deepLink: '/tasks',
    key: 'growth_task',
    title: '成长任务提醒',
  },
  {
    content: '本周成长报告已生成，来看看你的状态变化和关键提醒。',
    deepLink: '/reports',
    key: 'weekly_report',
    title: '成长周报已生成',
  },
  {
    content: '会员专属洞察已准备好，进入 App 查看你的深度分析。',
    deepLink: '/reports',
    key: 'vip_insight',
    targetType: 'level',
    targetValue: 'vip',
    title: '会员专属洞察',
  },
];

export function buildPushAudienceCountParams(input: {
  targetType?: string;
  targetValue?: string;
}): PushAudienceCountParams {
  const targetType = input.targetType === 'level' ? 'level' : 'all';
  if (targetType !== 'level') return { targetType: 'all' };
  const targetValue = stringValue(input.targetValue);
  return isValidPushMemberLevel(targetValue)
    ? { targetType, targetValue }
    : { targetType };
}

export function audienceCountLabel(count?: number) {
  return typeof count === 'number' && Number.isFinite(count)
    ? `预计 ${count} 人`
    : '尚未预估';
}

export function audienceCountDetailLabel(result?: {
  deviceCount?: number;
  targetType?: string;
  targetValue?: string;
  userCount?: number;
}) {
  if (!result) return '尚未预估';
  const users = Number(result.userCount ?? 0);
  const devices = Number(result.deviceCount ?? 0);
  const target = audienceTargetLabel(result.targetType, result.targetValue);
  const suffix = target ? `（${target}）` : '';
  return `预计 ${Number.isFinite(users) ? users : 0} 人 / ${Number.isFinite(devices) ? devices : 0} 台设备${suffix}`;
}

export function formatNoPushAudienceMessage(result?: {
  deviceCount?: number;
  userCount?: number;
}) {
  if (!result) return '';
  const devices = Number(result.deviceCount ?? 0);
  if (!Number.isFinite(devices) || devices > 0) return '';
  return '当前没有可推送设备，请先在 App 端登录并完成推送设备注册后再测试';
}

function audienceTargetLabel(targetType?: string, targetValue?: string) {
  const type = stringValue(targetType);
  const value = stringValue(targetValue);
  if (!type) return '';
  if (type === 'all') return '全部用户';
  if (type === 'level') {
    const label =
      pushMemberLevelOptions.find((item) => item.value === value)?.label ||
      value ||
      '未指定等级';
    return `会员等级：${label}`;
  }
  return value ? `${type}：${value}` : type;
}
