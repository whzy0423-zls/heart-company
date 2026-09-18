import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const root = resolve(import.meta.dirname, '../..');
const read = (path: string) => readFileSync(resolve(root, path), 'utf8');

describe('teacher admin management contract', () => {
  it('registers a permission-gated teacher management route', () => {
    const route = read('router/routes/modules/teacher.ts');
    expect(route).toContain("authority: ['Teacher:List']");
    expect(route).toContain("path: '/teachers'");
    expect(route).toContain("import('#/views/teacher/index.vue')");
  });

  it('supports teacher profile fields, role binding, and administrator override', () => {
    const source = read('views/teacher/index.vue');
    for (const label of ['老师标识', '姓名', '头像', '封面', '简介', '客服配置', '线下服务']) {
      expect(source).toContain(label);
    }
    for (const token of ['绑定 App 用户为老师', '启用老师', '管理员修改优先']) {
      expect(source).toContain(token);
    }
  });

  it('keeps review actions and required rejection reason visible', () => {
    const source = read('views/teacher/index.vue');
    for (const token of ['审核队列', '通过', '退回', '退回原因', '下架']) {
      expect(source).toContain(token);
    }
  });

  it('adds teacher, feed type, and review status filters to classroom management', () => {
    const api = read('api/core/classroom.ts');
    const source = read('views/classroom/index.vue');
    expect(api).toContain('teacherKey?: string');
    expect(api).toContain('feedType?:');
    expect(api).toContain('reviewStatus?:');
    for (const token of ['老师筛选', '正式课程', '日常动态', '审核状态']) {
      expect(source).toContain(token);
    }
  });
});
