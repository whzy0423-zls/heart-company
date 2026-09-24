import { describe, expect, it } from 'vitest';

import {
  getRequestErrorMessage,
  isSuccessfulResponseCode,
} from './request-error';

describe('request error handling', () => {
  it('supports both legacy and platform success codes', () => {
    expect(isSuccessfulResponseCode(0)).toBe(true);
    expect(isSuccessfulResponseCode(200)).toBe(true);
    expect(isSuccessfulResponseCode('200')).toBe(true);
    expect(isSuccessfulResponseCode(400)).toBe(false);
  });

  it('uses the backend msg for a business failure', () => {
    expect(
      getRequestErrorMessage({
        response: { data: { code: -450, msg: '资产状态不允许此操作' } },
      }),
    ).toBe('资产状态不允许此操作');
  });

  it('reads nested data and result messages', () => {
    expect(
      getRequestErrorMessage({
        response: { data: { code: 422, data: { message: '参数校验失败' } } },
      }),
    ).toBe('参数校验失败');
    expect(
      getRequestErrorMessage({
        response: { data: { code: 422, result: { msg: '保存失败' } } },
      }),
    ).toBe('保存失败');
  });

  it('keeps the code visible when the backend omits a message', () => {
    expect(
      getRequestErrorMessage(
        { response: { data: { code: 503 } } },
        '服务暂不可用',
      ),
    ).toBe('服务暂不可用（业务码：503）');
  });
});
