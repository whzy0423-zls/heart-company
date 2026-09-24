import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const here = dirname(fileURLToPath(import.meta.url));

describe('App agent discounts admin page', () => {
  it('registers under App management with plan view permission', () => {
    const routes = readFileSync(
      resolve(here, '../../router/routes/modules/app.ts'),
      'utf8',
    );
    expect(routes).toContain("title: '代理购卡优惠'");
    expect(routes).toContain("path: 'agent-discounts'");
    expect(routes).toContain("authority: ['App:PlanManagement:View']");
  });

  it('shows baseline and both audience prices for the six paid plans', () => {
    const page = readFileSync(resolve(here, 'agent-discounts.vue'), 'utf8');
    for (const expected of [
      'vip_month',
      'vip_quarter',
      'vip_year',
      'svip_month',
      'svip_quarter',
      'svip_year',
      '原价',
      '一级代理本人',
      '受邀用户',
      '优惠价',
      'getAppPlansApi',
      'getAppAgentDiscountsApi',
      'updateAppAgentDiscountsApi',
    ]) {
      expect(page).toContain(expected);
    }
  });

  it('allows write actions only with the existing plan write permission', () => {
    const page = readFileSync(resolve(here, 'agent-discounts.vue'), 'utf8');
    expect(page).toContain('App:PlanManagement:Write');
    expect(page).toContain('getAccessCodesApi');
    expect(page).toContain('Segmented');
    expect(page).toContain('InputNumber');
    expect(page).toContain('Switch');
    expect(page).toContain('只读模式');
  });
});
