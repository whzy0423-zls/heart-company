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
});
