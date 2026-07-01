import type { SignupLead } from '#/api';

import { describe, expect, it } from 'vitest';

import { getSignupNotifications } from './signup-notice';

function lead(
  input: Partial<SignupLead> &
    Pick<SignupLead, 'contact' | 'contactType' | 'createTime' | 'id' | 'name'>,
): SignupLead {
  return {
    contact: input.contact,
    contactType: input.contactType,
    createTime: input.createTime,
    followNote: '',
    followStatus: 'pending',
    gameResultId: '',
    id: input.id,
    interest: input.interest ?? '',
    ip: '',
    landingPage: '',
    message: '',
    name: input.name,
    nextFollowTime: '',
    owner: '',
    referrer: '',
    sourcePath: '',
    userAgent: '',
    utmCampaign: '',
    utmContent: '',
    utmMedium: '',
    utmSource: '',
    utmTerm: '',
    visitorId: '',
  };
}

const leads = [
  lead({
    contact: 'wx_new',
    contactType: 'wechat',
    createTime: '2026/06/17 18:40:00',
    id: '12',
    interest: '通知测试',
    name: '新报名',
  }),
  lead({
    contact: '13800000000',
    contactType: 'phone',
    createTime: '2026/06/17 18:39:00',
    id: '11',
    name: '旧报名',
  }),
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
