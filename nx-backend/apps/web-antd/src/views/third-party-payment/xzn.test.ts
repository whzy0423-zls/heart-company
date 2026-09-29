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
    expect(source).not.toContain(
      "{ label: 'MD5+RSA（推荐）', value: 'MD5+RSA' }",
    );
    expect(source).toContain('>MD5</Descriptions.Item>');
  });
});

describe('XZN H5 payment configuration boundary', () => {
  it('keeps the H5 return address separate from the native App return address', () => {
    expect(source).toContain("webReturnURL: ''");
    expect(source).toContain('v-model:value="paymentConfig.webReturnURL"');
    expect(source).toContain('v-model:value="paymentConfig.returnURL"');
    expect(source).toContain('label="H5 支付返回地址"');
    expect(source).toContain('label="App 同步返回地址"');
    expect(source).not.toMatch(
      /paymentConfig\.returnURL\s*=\s*paymentConfig\.webReturnURL/,
    );
    expect(source).not.toMatch(
      /paymentConfig\.webReturnURL\s*=\s*paymentConfig\.returnURL/,
    );
  });

  it('loads and saves the H5 field with the existing merchant configuration', () => {
    expect(source).toContain(
      "Object.assign(paymentConfig, data, { secret: '' })",
    );
    expect(source).toContain("paymentConfig.webReturnURL ||= ''");
    expect(source).toContain(
      "requestClient.put('/admin/xzn-pay/config', paymentConfig)",
    );
  });

  it('documents the trusted HTTPS billing return page without changing App mode', () => {
    expect(source).toContain(
      'https://.../h5/index.html#/billing?payment_return=1',
    );
    expect(source).toContain('必须使用 HTTPS');
    expect(source).toContain(
      '清空后保存将移除该独立 H5 返回地址配置；原生 App 返回地址不受影响',
    );
    expect(source).not.toContain('留空保留已有配置');
    expect(source).toContain('H5 会员购买直接使用 XZN');
    expect(source).toContain('App 继续按独立支付模式配置运行');
    expect(source).toContain('不会把 App 的客服微信模式切换为在线支付');
    expect(source).not.toContain('/admin/app-payment-mode');
    expect(source).not.toContain("mode: 'xzn'");
  });

  it('labels channel switches as shared XZN configuration rather than App-only', () => {
    expect(source).toContain('title="XZN 支付开关与网关（H5 / App 共用）"');
    expect(source).toContain('label="XZN 支付总开关"');
    expect(source).toContain('关闭后 H5 在线支付不可用');
  });
});
