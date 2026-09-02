import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const source = readFileSync(resolve(__dirname, 'xzn.vue'), 'utf8');

describe('XZN App payment configuration', () => {
  it('configures the total, Alipay, and WeChat switches independently', () => {
    expect(source).toContain('paymentConfig.enabled');
    expect(source).toContain('paymentConfig.alipayEnabled');
    expect(source).toContain('paymentConfig.wechatEnabled');
    expect(source).toContain('paymentConfig.alipayGatewayId');
    expect(source).toContain('paymentConfig.wechatGatewayId');
  });

  it('blocks the QR and JSAPI gateways from App WeChat payments', () => {
    expect(source).toContain("new Set(['3', '31'])");
    expect(source).toContain('不能作为 App 内微信支付网关');
    expect(source).toContain('请保持微信支付关闭');
  });

  it('keeps the admin signature mode MD5-only', () => {
    expect(source).toContain("paymentConfig.signType = 'MD5'");
    expect(source).toContain('星之柠接口固定使用 MD5 签名');
    expect(source).toContain('<Input value="MD5" disabled />');
    expect(source).not.toContain("{ label: 'RSA', value: 'RSA' }");
    expect(source).not.toContain("{ label: 'MD5+RSA（推荐）', value: 'MD5+RSA' }");
    expect(source).toContain('>MD5</Descriptions.Item>');
  });
});
