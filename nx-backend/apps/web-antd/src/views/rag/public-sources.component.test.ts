import { createApp, nextTick } from 'vue';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const api = vi.hoisted(() => ({
  list: vi.fn(),
  preview: vi.fn(),
  status: vi.fn(),
}));
vi.mock('#/api', () => ({
  getPublicKnowledgeSourceChunksApi: api.preview,
  getPublicKnowledgeSourcesApi: api.list,
  updatePublicKnowledgeSourcesStatusApi: api.status,
}));
vi.mock('#/components/ellipsis-tooltip/ellipsis-tooltip.vue', () => ({
  default: { props: ['text'], template: '<span>{{ text }}</span>' },
}));
vi.mock('#/components/ellipsis-tooltip/table', () => ({
  ellipsisColumn: (
    dataIndex: string,
    title: string,
    options: Record<string, unknown>,
  ) => ({ dataIndex, title, ...options }),
}));

import PublicSources from './public-sources.vue';

const source = {
  category: '心理学',
  createTime: '',
  datasetId: 'fixture',
  enabled: true,
  extractStatus: 'done',
  fileFormat: 'pdf',
  id: 'fixture:book:1',
  importedChunks: 2,
  qualityStatus: 'ready',
  sourceChunks: 4,
  sourceKind: 'book',
  sourceRecordId: 1,
  textChars: 100,
  title: '关系心理学',
  updateTime: '',
};
let unmount: (() => void) | undefined;

async function settle() {
  await new Promise((resolve) => setTimeout(resolve, 30));
  await nextTick();
}

async function mountDirectory() {
  const root = document.createElement('div');
  document.body.append(root);
  const app = createApp(PublicSources);
  app.mount(root);
  unmount = () => app.unmount();
  await settle();
  return root;
}

describe('mounted public source directory', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue({
      categories: [{ count: 1, name: '心理学' }],
      items: [{ ...source }],
      page: 1,
      pageSize: 20,
      total: 1,
    });
    api.preview.mockResolvedValue({
      items: [
        {
          id: 'chunk:1',
          title: '沟通',
          content: '成长正文',
          locator: { page_start: 3, source_ref: '/private/book.pdf' },
          importBatchId: 'batch:1',
        },
      ],
      total: 2,
    });
  });

  afterEach(() => {
    unmount?.();
    document.body.innerHTML = '';
  });

  it('renders the catalog and an imported-body preview without filesystem provenance', async () => {
    const root = await mountDirectory();
    expect(root.textContent).toContain('关系心理学');
    expect(root.textContent).toContain('2 / 4');
    (
      root.querySelector('[aria-label="预览关系心理学"]') as HTMLElement
    ).click();
    await settle();
    expect(api.preview).toHaveBeenCalledWith(source.id);
    expect(document.body.textContent).toContain('成长正文');
    expect(document.body.textContent).toContain('起始页：3');
    expect(document.body.textContent).not.toContain('/private/book.pdf');
  });

  it('keeps a row switch enabled after a failed update and renders retry', async () => {
    api.status.mockRejectedValue(new Error('目录状态更新失败'));
    const root = await mountDirectory();
    const toggle = root.querySelector(
      '[aria-label="关系心理学启用状态"]',
    ) as HTMLElement;
    toggle.click();
    await settle();
    expect(api.status).toHaveBeenCalledWith({
      enabled: false,
      ids: [source.id],
    });
    expect(toggle.getAttribute('aria-checked')).toBe('true');
    expect(root.textContent).toContain('目录状态更新失败');
    expect(root.textContent?.replaceAll(/\s/g, '')).toContain('重试');
  });
});
