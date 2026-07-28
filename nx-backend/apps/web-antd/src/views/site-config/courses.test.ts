import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';

import { parse } from 'vue/compiler-sfc';
import { describe, expect, it } from 'vitest';

function editorShellProps(source: string) {
  const { descriptor } = parse(source);
  const ast = descriptor.template?.ast;
  expect(ast).toBeTruthy();
  if (!ast) throw new Error('courses template AST is missing');
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

function renderedText(source: string) {
  const { descriptor } = parse(source);
  const ast = descriptor.template?.ast;
  expect(ast).toBeTruthy();
  if (!ast) throw new Error('courses template AST is missing');
  const texts: string[] = [];
  const visit = (node: any) => {
    if (node.type === 2) texts.push(node.content.trim());
    node.children?.forEach(visit);
  };
  ast.children.forEach(visit);
  return texts.filter(Boolean);
}

describe('course site-config editor terminology', () => {
  it('renders course-product labels and separates classroom media guidance', async () => {
    const source = await readFile(
      resolve('apps/web-antd/src/views/site-config/courses.vue'),
      'utf8',
    );

    expect(editorShellProps(source)).toMatchObject({
      description:
        '配置课程方向与报名产品，不管理课堂音视频内容；视频、音频课件请前往「老师课堂」。',
      title: '课程产品管理',
    });
    expect(renderedText(source)).toContain('课程产品卡片');
    expect(renderedText(source)).toContain('新增课程产品');
  });
});
