import type { VNode } from 'vue';

import { h } from 'vue';

import EllipsisTooltip from './ellipsis-tooltip.vue';

export interface EllipsisColumnOptions {
  fixed?: 'left' | 'right';
  key?: string;
  lines?: number;
  maxWidth?: number | string;
  width?: number | string;
}

export function ellipsisColumn(
  dataIndex: string,
  title: string,
  options: EllipsisColumnOptions = {},
) {
  const { lines = 1, maxWidth, ...columnOptions } = options;
  return {
    dataIndex,
    ellipsis: { showTitle: false },
    title,
    ...columnOptions,
    customRender: ({ text }: { text: unknown }): VNode =>
      h(EllipsisTooltip, { lines, maxWidth, text: text as any }),
  };
}
