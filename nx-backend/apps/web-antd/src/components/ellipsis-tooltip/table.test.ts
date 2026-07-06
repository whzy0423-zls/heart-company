import { describe, expect, it } from 'vitest';

import { ellipsisColumn } from './table';

describe('ellipsisColumn', () => {
  it('creates an ant table column that renders EllipsisTooltip', () => {
    const column = ellipsisColumn('content', '内容', { width: 240, lines: 2 });

    expect(column.dataIndex).toBe('content');
    expect(column.title).toBe('内容');
    expect(column.width).toBe(240);
    expect(column.ellipsis).toEqual({ showTitle: false });

    const vnode = column.customRender?.({ text: '完整内容' } as any) as any;
    expect(vnode.props.text).toBe('完整内容');
    expect(vnode.props.lines).toBe(2);
  });
});
