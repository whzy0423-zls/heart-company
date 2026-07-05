import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const __dirname = dirname(fileURLToPath(import.meta.url));

function source(relative: string) {
  return readFileSync(resolve(__dirname, relative), 'utf8');
}

const pages = [
  {
    api: 'getSystemUserListApi',
    file: './user/list.vue',
    name: 'user list',
    searchField: 'username',
  },
  {
    api: 'getSystemRoleListApi',
    file: './role/list.vue',
    name: 'role list',
    searchField: 'name',
  },
];

describe('system user/role list pagination and dangerous actions', () => {
  for (const page of pages) {
    it(`${page.name} sends pagination params and binds table pagination changes`, () => {
      const text = source(page.file);

      expect(text).toMatch(new RegExp(`${page.searchField}:\\s*''`));
      expect(text).toMatch(/page:\s*1/);
      expect(text).toMatch(/pageSize:\s*20/);
      expect(text).toMatch(
        new RegExp(
          `${page.api}\\(\\s*\\{[\\s\\S]*page:\\s*query\\.value\\.page[\\s\\S]*pageSize:\\s*query\\.value\\.pageSize`,
        ),
      );
      expect(text).toContain('function handleTableChange');
      expect(text).toContain('@change="handleTableChange"');
      expect(text).toContain('current: query.page');
      expect(text).toContain('pageSize: query.pageSize');
      expect(text).toContain('showSizeChanger: true');
    });

    it(`${page.name} confirms delete and disable operations while preserving permission-gated actions`, () => {
      const text = source(page.file);

      expect(text).toContain('Popconfirm');
      expect(text).toContain('@confirm="remove');
      expect(text).not.toContain('@click="remove');
      expect(text).toContain('v-if="canDelete"');
      expect(text).toContain('v-if="canUpdate"');
      expect(text).toContain('function handleStatusChange');
      expect(text).toContain('Modal.confirm');
      expect(text).toContain('确认停用');
      expect(text).toContain('@change="handleStatusChange"');
      expect(text).not.toContain('v-model:checked="form.status"');
    });
  }
});
