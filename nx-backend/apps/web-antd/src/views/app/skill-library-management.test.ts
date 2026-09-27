import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const here = dirname(fileURLToPath(import.meta.url));
const source = readFileSync(resolve(here, 'skill-library-management.vue'), 'utf8');

describe('growth skill library management contract', () => {
  it('is mounted below App management with its own permissions', () => {
    const routes = readFileSync(resolve(here, '../../router/routes/modules/app.ts'), 'utf8');
    expect(routes).toContain("title: '成长技能库'");
    expect(routes).toContain("authority: ['App:SkillLibrary:View']");
  });

  it('keeps the legacy books route as an alias for the skill library page', () => {
    const routes = readFileSync(resolve(here, '../../router/routes/modules/app.ts'), 'utf8');
    expect(routes).toContain("path: 'skill-library'");
    expect(routes).toContain("alias: 'books'");
  });

  it('edits library, category and skill metadata without changing stable keys', () => {
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
      'publishSkillLibrarySkillApi',
      'unpublishSkillLibrarySkillApi',
      'enableSkillLibrarySkillApi',
      'disableSkillLibrarySkillApi',
      '发布',
      '下架',
      '启用',
      '停用',
      '技能版本和书籍数据',
      'App 走新技能库接口刷新生效',
    ]) {
      expect(source).toContain(expected);
    }
    expect(source).toContain('技能标识');
    expect(source).toContain('disabled');
  });

  it('places the book upload action in a prominent top panel', () => {
    expect(source).toContain('导入书籍，生成成长技能');
    expect(source).toContain('上传书籍并生成技能');
    expect(source).toContain('importOpen = true');
    expect(source).toContain('支持 TXT、Markdown、DOCX、EPUB、PDF');
  });
});
