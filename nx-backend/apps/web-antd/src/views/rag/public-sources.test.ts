import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it, vi } from 'vitest';

import { createPublicSourcesState, previewLocator } from './public-sources';

const source = {
  category: '心理学',
  createTime: '',
  datasetId: 'fixture',
  enabled: true,
  extractStatus: 'done',
  fileFormat: 'pdf',
  id: 'fixture:book:1',
  importedChunks: 2,
  qualityStatus: 'ready' as const,
  sourceChunks: 4,
  sourceKind: 'book' as const,
  sourceRecordId: 1,
  textChars: 100,
  title: '关系心理学',
  updateTime: '',
};

function fixtureApi() {
  return {
    chunks: vi.fn().mockResolvedValue({ items: [], total: 0 }),
    list: vi.fn().mockResolvedValue({
      categories: [{ count: 1, name: '心理学' }],
      items: [{ ...source }],
      page: 1,
      pageSize: 20,
      total: 1,
    }),
    status: vi.fn().mockResolvedValue({ updated: 1 }),
  };
}

describe('public source directory state', () => {
  it('loads categories, maps filters and resets pagination/selection on search', async () => {
    const api = fixtureApi();
    const state = createPublicSourcesState(api);
    state.query.enabled = 'disabled';
    state.query.category = '心理学';
    state.query.page = 3;
    state.selectedIds.value = [source.id];
    await state.search();
    expect(api.list).toHaveBeenCalledWith(
      expect.objectContaining({
        category: '心理学',
        enabled: false,
        page: 1,
        pageSize: 20,
      }),
    );
    expect(state.selectedIds.value).toEqual([]);
    expect(state.items.value[0]?.title).toBe(source.title);
    expect(state.categories.value).toEqual([{ count: 1, name: '心理学' }]);
    await state.changePage({ current: 2, pageSize: 50 });
    expect(api.list).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 2, pageSize: 50 }),
    );
  });

  it('keeps status unchanged when activation fails and permits retry', async () => {
    const api = fixtureApi();
    const state = createPublicSourcesState(api);
    await state.load();
    api.status.mockRejectedValueOnce(new Error('更新失败'));
    expect(await state.setEnabled([source.id], false)).toBe(false);
    expect(state.items.value[0]?.enabled).toBe(true);
    expect(state.error.value).toBe('更新失败');
    expect(state.saving.value).toBe(false);
    api.list.mockResolvedValueOnce({
      categories: [],
      items: [{ ...source, enabled: false }],
      page: 1,
      pageSize: 20,
      total: 1,
    });
    expect(await state.setEnabled([source.id], false)).toBe(true);
    expect(state.items.value[0]?.enabled).toBe(false);
    expect(api.status).toHaveBeenLastCalledWith({
      enabled: false,
      ids: [source.id],
    });
  });

  it('exposes an actionable error and clears stale items after failed loading', async () => {
    const api = fixtureApi();
    const state = createPublicSourcesState(api);
    await state.load();
    api.list.mockRejectedValueOnce(new Error('目录暂不可用'));
    await state.load();
    expect(state.items.value).toEqual([]);
    expect(state.error.value).toBe('目录暂不可用');
    expect(state.loading.value).toBe(false);
    await state.load();
    expect(state.error.value).toBe('');
    expect(state.total.value).toBe(1);
  });

  it('discards an older request that finishes after newer filters', async () => {
    const api = fixtureApi();
    let finishOld!: (result: any) => void;
    api.list.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finishOld = resolve;
        }),
    );
    const state = createPublicSourcesState(api);
    const old = state.load();
    await state.search();
    finishOld({ categories: [], items: [], page: 1, pageSize: 20, total: 0 });
    await old;
    expect(state.total.value).toBe(1);
  });

  it('previews imported chunks and clears previous source contents on failure', async () => {
    const api = fixtureApi();
    api.chunks.mockResolvedValueOnce({
      items: [
        {
          id: 'chunk:1',
          title: '第一章',
          content: '正文',
          locator: { page: 3 },
          importBatchId: 'batch:1',
        },
      ],
      total: 2,
    });
    const state = createPublicSourcesState(api);
    await state.preview(source);
    expect(state.previewItems.value[0]?.content).toBe('正文');
    expect(state.previewTotal.value).toBe(2);
    api.chunks.mockRejectedValueOnce(new Error('预览暂不可用'));
    await state.preview({ ...source, id: 'fixture:book:2' });
    expect(state.previewItems.value).toEqual([]);
    expect(state.previewError.value).toBe('预览暂不可用');
    expect(state.previewLoading.value).toBe(false);
  });

  it('shows only bounded provenance fields, never file paths or arbitrary metadata', () => {
    const rendered = previewLocator({
      chapter: '第一章',
      page: 3,
      chunkId: 22,
      path: '/private/book.pdf',
      filePath: 'C:\\books\\one.pdf',
      token: 'secret',
      title: '<script>',
    });
    expect(rendered).toContain('第一章');
    expect(rendered).toContain('3');
    expect(rendered).not.toMatch(/private|books|secret|script/);
  });

  it('preserves actual SQLite locator keys and only relative EPUB references', () => {
    const rendered = previewLocator({
      page_start: 12,
      page_end: 14,
      section_title: '沟通与成长',
      sqlite_chunk_id: 18,
      source_chunk_id: 19,
      segment_index: 2,
      source_ref: 'text/ch01.xhtml#intro',
    });
    expect(rendered).toContain('12');
    expect(rendered).toContain('14');
    expect(rendered).toContain('沟通与成长');
    expect(rendered).toContain('18');
    expect(rendered).toContain('text/ch01.xhtml#intro');
    for (const source_ref of [
      '/private/books/one.pdf',
      'C:\\books\\one.epub',
      '../secret/ch01.xhtml',
      'file:///tmp/ch01.xhtml',
    ]) {
      expect(previewLocator({ source_ref })).toBe('');
    }
  });
});

describe('public directory UI integration', () => {
  it('adds a source tab while retaining manual knowledge controls', () => {
    const page = readFileSync(resolve(__dirname, 'knowledge.vue'), 'utf8');
    expect(page).toContain('书籍目录');
    expect(page).toContain('<PublicSources');
    expect(page).toContain('openCreate');
    expect(page).toContain('handleReindex');
  });

  it('provides preview, batch activation, errors and responsive table overflow', () => {
    const component = readFileSync(
      resolve(__dirname, 'public-sources.vue'),
      'utf8',
    );
    for (const token of [
      '批量启用',
      '批量停用',
      'row-selection',
      'Switch',
      'Drawer',
      'previewLocator',
      'qualityStatus',
      'showSizeChanger',
      'Alert',
      '重试',
      'scroll',
      'max-width: 768px',
    ]) {
      expect(component).toContain(token);
    }
    expect(component).not.toContain('v-html');
    expect(component).not.toContain('JSON.stringify');
  });
});
