import type {
  PublicKnowledgeSource,
  PublicKnowledgeSourceChunk,
  PublicKnowledgeSourcesQuery,
  PublicKnowledgeSourcesResult,
} from '../../api/core/rag';

import { reactive, ref } from 'vue';

interface SourceApi {
  chunks: (
    id: string,
  ) => Promise<{ items: PublicKnowledgeSourceChunk[]; total: number }>;
  list: (
    query: PublicKnowledgeSourcesQuery,
  ) => Promise<PublicKnowledgeSourcesResult>;
  status: (data: {
    enabled: boolean;
    ids: string[];
  }) => Promise<{ updated: number }>;
}

function errorText(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

export function createPublicSourcesState(api: SourceApi) {
  const items = ref<PublicKnowledgeSource[]>([]);
  const categories = ref<PublicKnowledgeSourcesResult['categories']>([]);
  const total = ref(0);
  const loading = ref(false);
  const saving = ref(false);
  const error = ref('');
  const selectedIds = ref<string[]>([]);
  const pendingIds = ref<string[]>([]);
  const query = reactive({
    category: '',
    datasetId: '',
    enabled: '',
    keyword: '',
    page: 1,
    pageSize: 20,
    qualityStatus: '',
  });
  const previewSource = ref<PublicKnowledgeSource>();
  const previewOpen = ref(false);
  const previewLoading = ref(false);
  const previewError = ref('');
  const previewItems = ref<PublicKnowledgeSourceChunk[]>([]);
  const previewTotal = ref(0);
  let loadVersion = 0;
  let previewVersion = 0;

  async function load() {
    const version = ++loadVersion;
    loading.value = true;
    error.value = '';
    selectedIds.value = [];
    try {
      const result = await api.list({
        category: query.category || undefined,
        datasetId: query.datasetId.trim() || undefined,
        enabled: query.enabled === '' ? undefined : query.enabled === 'enabled',
        keyword: query.keyword.trim() || undefined,
        page: query.page,
        pageSize: query.pageSize,
        qualityStatus: query.qualityStatus || undefined,
      });
      if (version !== loadVersion) return;
      items.value = result.items ?? [];
      categories.value = result.categories ?? [];
      total.value = result.total;
    } catch (cause) {
      if (version !== loadVersion) return;
      items.value = [];
      total.value = 0;
      error.value = errorText(cause, '书籍目录加载失败');
    } finally {
      if (version === loadVersion) loading.value = false;
    }
  }

  async function search() {
    query.page = 1;
    await load();
  }

  async function changePage(pagination: {
    current?: number;
    pageSize?: number;
  }) {
    query.page = pagination.current ?? 1;
    query.pageSize = pagination.pageSize ?? 20;
    await load();
  }

  async function setEnabled(ids: string[], enabled: boolean) {
    if (ids.length === 0 || saving.value) return false;
    saving.value = true;
    pendingIds.value = [...ids];
    error.value = '';
    try {
      await api.status({ enabled, ids: [...ids] });
      for (const source of items.value) {
        if (ids.includes(source.id)) source.enabled = enabled;
      }
      if (previewSource.value && ids.includes(previewSource.value.id)) {
        previewSource.value.enabled = enabled;
      }
      await load();
      return true;
    } catch (cause) {
      error.value = errorText(cause, '书籍启用状态更新失败');
      return false;
    } finally {
      saving.value = false;
      pendingIds.value = [];
    }
  }

  async function preview(source: PublicKnowledgeSource) {
    const version = ++previewVersion;
    previewSource.value = source;
    previewOpen.value = true;
    previewLoading.value = true;
    previewError.value = '';
    previewItems.value = [];
    previewTotal.value = 0;
    try {
      const result = await api.chunks(source.id);
      if (version !== previewVersion) return;
      previewItems.value = result.items ?? [];
      previewTotal.value = result.total;
    } catch (cause) {
      if (version === previewVersion)
        previewError.value = errorText(cause, '正文预览加载失败');
    } finally {
      if (version === previewVersion) previewLoading.value = false;
    }
  }

  return {
    categories,
    changePage,
    error,
    items,
    load,
    loading,
    pendingIds,
    preview,
    previewError,
    previewItems,
    previewLoading,
    previewOpen,
    previewSource,
    previewTotal,
    query,
    saving,
    search,
    selectedIds,
    setEnabled,
    total,
  };
}

const locatorLabels: Record<string, string> = {
  chunk_index: '原片段序号',
  chapter: '章节',
  chapterTitle: '章节',
  chunkId: '原片段',
  cleaningVersion: '清洗版本',
  page: '页码',
  pageEnd: '末页',
  pageStart: '起始页',
  page_start: '起始页',
  page_end: '末页',
  section_id: '章节编号',
  section_title: '章节',
  segment_index: '分片序号',
  source_chunk_id: '原片段',
  source_ref: 'EPUB 章节',
  sourceChunkId: '原片段',
  sqlite_chunk_id: 'SQLite 原片段',
};

export function previewLocator(locator: Record<string, unknown>) {
  return Object.entries(locatorLabels)
    .flatMap(([key, label]) => {
      const value = locator[key];
      if (typeof value !== 'number' && typeof value !== 'string') return [];
      const text = String(value).slice(0, 160);
      if (/^(?:\/|[a-z]:[\\/]|file:)/i.test(text)) return [];
      if (
        key === 'source_ref' &&
        (text.includes('..') || !/^[\w./-]+\.x?html(?:#[\w.-]+)?$/i.test(text))
      )
        return [];
      return text ? [`${label}：${text}`] : [];
    })
    .join(' · ');
}
