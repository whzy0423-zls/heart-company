import { describe, expect, it } from 'vitest';

import { buildMessageListParams } from './management-query';

describe('buildMessageListParams', () => {
  it('filters registration messages by related signup business', () => {
    expect(
      buildMessageListParams({
        category: 'signup',
        keyword: '',
        page: 1,
        pageSize: 20,
        read: '',
      }),
    ).toEqual({
      businessType: 'signup',
      keyword: '',
      page: 1,
      pageSize: 20,
      read: undefined,
    });
  });

  it('does not apply a business filter for all messages', () => {
    expect(
      buildMessageListParams({
        category: '',
        keyword: '预约',
        page: 2,
        pageSize: 50,
        read: 'false',
      }),
    ).toEqual({
      businessType: undefined,
      keyword: '预约',
      page: 2,
      pageSize: 50,
      read: 'false',
    });
  });
});
