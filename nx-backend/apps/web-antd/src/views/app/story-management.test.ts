import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const here = dirname(fileURLToPath(import.meta.url));

describe('story skill management CRUD contract', () => {
  it('uses an available story icon in the App management menu', () => {
    const routes = readFileSync(resolve(here, '../../router/routes/modules/app.ts'), 'utf8');
    expect(routes).toContain("icon: 'lucide:book-open-text'");
  });

  it('exposes upload, detail, edit, delete and publish actions', () => {
    const source = readFileSync(resolve(here, 'story-management.vue'), 'utf8');
    for (const expected of [
      '上传 Skill',
      '查看详情',
      '编辑 Skill',
      '删除 Skill',
      '发布到 App',
      'getStorySkillApi',
      'updateStorySkillApi',
      'deleteStorySkillApi',
      'App:StoryManagement:Delete',
    ]) {
      expect(source).toContain(expected);
    }
  });

  it('only exposes edit for skills without a published version', () => {
    const source = readFileSync(resolve(here, 'story-management.vue'), 'utf8');
    expect(source).toContain('canEdit && !record.publishedVersion');
    expect(source).toContain('canEdit && !item.publishedVersion');
  });

  it('keeps create as an upload and exposes detail/update/delete APIs', () => {
    const source = readFileSync(resolve(here, '../../api/core/story-skill.ts'), 'utf8');
    expect(source).toContain("requestClient.upload<StorySkillAdminItem>('/story-skills/upload'");
    expect(source).toContain('getStorySkillApi');
    expect(source).toContain('updateStorySkillApi');
    expect(source).toContain('deleteStorySkillApi');
    expect(source).toContain('requestClient.delete');
  });

  it('exposes an isolated story-generation model configuration', () => {
    const source = readFileSync(resolve(here, 'story-management.vue'), 'utf8');
    for (const expected of [
      '故事生成模型',
      'getStoryGenerationConfigApi',
      'updateStoryGenerationConfigApi',
      'testStoryGenerationConfigApi',
      '仅用于“我的故事”生成',
    ]) {
      expect(source).toContain(expected);
    }
  });
});
