import { createApp } from 'vue';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('ant-design-vue', async () => import('#/test-utils/antd-stubs'));

import EllipsisTooltip from './ellipsis-tooltip.vue';

let host: HTMLDivElement | null = null;

afterEach(() => {
  host?.remove();
  host = null;
});

function renderComponent(props: Record<string, any>) {
  host = document.createElement('div');
  document.body.append(host);
  createApp(EllipsisTooltip, props).mount(host);
  return host;
}

describe('EllipsisTooltip', () => {
  it('renders full text into tooltip title and visible truncated text', () => {
    const root = renderComponent({ text: '一段非常长的内容，需要省略但 hover 展示完整内容' });

    expect(root.querySelector('.tooltip-stub')?.getAttribute('data-title')).toBe(
      '一段非常长的内容，需要省略但 hover 展示完整内容',
    );
    expect(root.querySelector('.ellipsis-tooltip__text')?.textContent).toBe(
      '一段非常长的内容，需要省略但 hover 展示完整内容',
    );
    expect(root.querySelector('.ellipsis-tooltip__text')?.className).toContain(
      'ellipsis-tooltip__text--single',
    );
  });

  it('supports two-line clamped text', () => {
    const root = renderComponent({ lines: 2, text: '第一行第二行第三行第四行' });

    expect(root.querySelector('.ellipsis-tooltip__text')?.className).toContain(
      'ellipsis-tooltip__text--multi',
    );
    expect(root.querySelector('.ellipsis-tooltip__text')?.getAttribute('style')).toContain(
      '--ellipsis-lines: 2',
    );
  });

  it('uses fallback text when value is empty', () => {
    const root = renderComponent({ emptyText: '-', text: '' });

    expect(root.querySelector('.tooltip-stub')?.getAttribute('data-title')).toBe('-');
    expect(root.textContent).toBe('-');
  });

  it('forwards custom class to the visible truncated text element', () => {
    const root = renderComponent({ class: 'custom-cell-name', text: '资产名称' });

    expect(root.querySelector('.ellipsis-tooltip__text')?.className).toContain('custom-cell-name');
  });
});
