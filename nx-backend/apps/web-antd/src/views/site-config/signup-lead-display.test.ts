import { describe, expect, it } from 'vitest';

import {
  enrollmentKindLabel,
  parseTeacherEnrollment,
} from './signup-lead-display';

describe('teacher enrollment display', () => {
  it('parses the structured teacher enrollment fields from a signup lead', () => {
    const details = parseTeacherEnrollment({
      interest: '老师：韩老师（han） | 报名类型：consult | 意向方向：个人成长',
      message:
        '老师：韩老师（han）\n报名类型：consult\n意向方向：个人成长\n期望时间：下周六上午\n留言：希望了解课程安排',
      sourcePlatform: 'website',
    });

    expect(details).toEqual({
      isTeacherEnrollment: true,
      teacherKey: 'han',
      teacherName: '韩老师',
      kind: 'consult',
      intent: '个人成长',
      preferredTime: '下周六上午',
      message: '希望了解课程安排',
    });
    expect(enrollmentKindLabel(details.kind)).toBe('1v1 咨询预约');
  });

  it('keeps legacy website leads displayable without inventing enrollment fields', () => {
    expect(
      parseTeacherEnrollment({
        interest: '课程咨询',
        message: '想了解课程',
        sourcePlatform: 'website',
      }),
    ).toEqual({
      isTeacherEnrollment: false,
      teacherKey: '',
      teacherName: '',
      kind: '',
      intent: '',
      preferredTime: '',
      message: '',
    });
  });
});
