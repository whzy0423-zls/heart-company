import type { NotificationItem } from '@vben/layouts';

import type { SignupLead } from '#/api';

export const SIGNUP_NOTICE_AVATAR = '/favicon.png';

export function contactTypeLabel(type?: string) {
  return type === 'wechat' ? '微信号' : '手机号';
}

export function toSignupNotification(item: SignupLead): NotificationItem {
  return {
    avatar: SIGNUP_NOTICE_AVATAR,
    date: item.createTime,
    id: `signup-${item.id}`,
    isRead: false,
    link: '/message/management',
    message: `${item.name} / ${contactTypeLabel(item.contactType)}: ${item.contact}${item.interest ? ` / ${item.interest}` : ''}`,
    query: { type: 'signup' },
    title: '新的报名信息',
  };
}

export function getSignupNotifications(
  items: SignupLead[],
  lastSeenId: number,
): { latestId: number; notices: NotificationItem[] } {
  if (items.length === 0) {
    return { latestId: lastSeenId, notices: [] };
  }

  const latestId = Math.max(...items.map((item) => Number(item.id) || 0));
  const normalizedLastSeenId = Number.isFinite(lastSeenId) ? lastSeenId : 0;
  const freshItems =
    normalizedLastSeenId === 0 || normalizedLastSeenId > latestId
      ? items.slice(0, 1)
      : items.filter((item) => Number(item.id) > normalizedLastSeenId);

  return {
    latestId,
    notices: freshItems
      .toSorted((a, b) => Number(a.id) - Number(b.id))
      .map((item) => toSignupNotification(item)),
  };
}
