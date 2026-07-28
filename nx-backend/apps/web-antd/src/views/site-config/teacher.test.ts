import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';

import { parse } from 'vue/compiler-sfc';
import { describe, expect, it } from 'vitest';

function editorShellProps(source: string) {
  const { descriptor } = parse(source);
  const ast = descriptor.template?.ast;
  expect(ast).toBeTruthy();
  if (!ast) throw new Error('teacher template AST is missing');
  const shell = ast.children.find(
    (node: any) => node.type === 1 && node.tag === 'EditorShell',
  ) as any;
  expect(shell).toBeTruthy();
  return Object.fromEntries(
    shell.props
      .filter((prop: any) => prop.type === 6)
      .map((prop: any) => [prop.name, prop.value?.content ?? true]),
  );
}

describe('teacher site-config editor terminology', () => {
  it('renders profile-management guidance separately from classroom media', async () => {
    const source = await readFile(
      resolve('apps/web-antd/src/views/site-config/teacher.vue'),
      'utf8',
    );

    expect(editorShellProps(source)).toMatchObject({
      description:
        '配置官网与小程序展示的老师简介；课堂音视频内容及其老师快照请前往「老师课堂」管理。',
      title: '老师资料管理',
    });
  });
});
