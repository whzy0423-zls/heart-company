import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const here = dirname(fileURLToPath(import.meta.url));

describe('App plan management contract', () => {
  it('registers plan management under App management', () => {
    const source = readFileSync(
      resolve(here, '../../router/routes/modules/app.ts'),
      'utf8',
    );
    expect(source).toContain("title: '套餐管理'");
    expect(source).toContain("authority: ['App:PlanManagement:View']");
    expect(source).toContain("path: 'plan-management'");
  });

  it('exposes list and update APIs', () => {
    const source = readFileSync(
      resolve(here, '../../api/core/app-plan.ts'),
      'utf8',
    );
    expect(source).toContain(
      "requestClient.get<AppPlan[]>('/admin/app-plans')",
    );
    expect(source).toContain('requestClient.put<AppPlan>');
  });

  it('edits pricing, availability and quota fields', () => {
    const source = readFileSync(resolve(here, 'plan-management.vue'), 'utf8');
    for (const expected of [
      '套餐价格',
      '划线价格',
      '每日聊天额度',
      '每月故事额度',
      '人物卡上限',
      '上架状态',
      'App:PlanManagement:Write',
      'updateAppPlanApi',
    ]) {
      expect(source).toContain(expected);
    }
  });

  it('edits normalized membership level and billing cycle fields', () => {
    const source = readFileSync(resolve(here, 'plan-management.vue'), 'utf8');
    for (const expected of [
      '会员等级',
      '购买周期',
      'planLevel',
      'billingCycle',
      'featuresJson',
      'limitsJson',
      'read_only_over_limit',
    ]) {
      expect(source).toContain(expected);
    }
  });

  it('keeps free plan validation visible in the editor', () => {
    const source = readFileSync(resolve(here, 'plan-management.vue'), 'utf8');
    expect(source).toContain('免费版只能使用 none 周期');
    expect(source).toContain('付费套餐必须选择 VIP 或 S VIP');
  });

  it('refreshes current permissions before exposing write actions', () => {
    const source = readFileSync(resolve(here, 'plan-management.vue'), 'utf8');
    expect(source).toContain('getAccessCodesApi');
    expect(source).toContain('access.setAccessCodes');
    expect(source).toContain('只读模式');
  });

  it('provides explicit edit and availability actions with confirmation', () => {
    const source = readFileSync(resolve(here, 'plan-management.vue'), 'utf8');
    for (const expected of [
      'toggleAvailability',
      'Modal.confirm',
      'actionLoadingCode',
      "recordOf(record).enabled ? '下架' : '上架'",
      '>编辑<',
    ]) {
      expect(source).toContain(expected);
    }
  });
});
