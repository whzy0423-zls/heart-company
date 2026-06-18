import { describe, expect, it } from 'vitest';

import { getSignupNotifications } from './signup-notice';

const leads = [
  {
    contact: 'wx_new',
    contactType: 'wechat',
    createTime: '2026/06/17 18:40:00',
    id: '12',
    interest: '通知测试',
    ip: '',
    message: '',
    name: '新报名',
    userAgent: '',
  },
  {
    contact: '13800000000',
    contactType: 'phone',
    createTime: '2026/06/17 18:39:00',
    id: '11',
    interest: '',
    ip: '',
    message: '',
    name: '旧报名',
    userAgent: '',
  },
];

describe('signup notice', () => {
  it('creates a notification for the latest signup on first load', () => {
    const result = getSignupNotifications(leads, 0);

    expect(result.latestId).toBe(12);
    expect(result.notices).toHaveLength(1);
    expect(result.notices[0]?.id).toBe('signup-12');
    expect(result.notices[0]?.message).toContain('微信号: wx_new');
  });

  it('creates notifications only for items newer than the last seen id', () => {
    const result = getSignupNotifications(leads, 11);

    expect(result.latestId).toBe(12);
    expect(result.notices).toHaveLength(1);
    expect(result.notices[0]?.id).toBe('signup-12');
  });

  it('recovers when stored last seen id is newer than server data', () => {
    const result = getSignupNotifications(leads, 999);

    expect(result.latestId).toBe(12);
    expect(result.notices).toHaveLength(1);
    expect(result.notices[0]?.id).toBe('signup-12');
  });

  it('recovers when stored last seen id is invalid', () => {
    const result = getSignupNotifications(leads, Number.NaN);

    expect(result.latestId).toBe(12);
    expect(result.notices).toHaveLength(1);
    expect(result.notices[0]?.id).toBe('signup-12');
  });
});
