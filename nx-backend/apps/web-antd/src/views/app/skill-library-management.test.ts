import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const here = dirname(fileURLToPath(import.meta.url));

describe('growth skill library management contract', () => {
  it('is mounted below App management with its own permissions', () => {
    const routes = readFileSync(resolve(here, '../../router/routes/modules/app.ts'), 'utf8');
    expect(routes).toContain("title: '成长技能库'");
    expect(routes).toContain("authority: ['App:SkillLibrary:View']");
  });

  it('edits library, category and skill metadata without changing stable keys', () => {
    const source = readFileSync(resolve(here, 'skill-library-management.vue'), 'utf8');
    for (const expected of [
      '技能名称',
      '技能简介',
      '所属分类',
      '图标',
      '颜色',
      '排序',
      '启用状态',
      '编辑技能',
      'updateSkillLibrarySkillApi',
    ]) {
      expect(source).toContain(expected);
    }
    expect(source).toContain('技能标识');
    expect(source).toContain('disabled');
  });
});
