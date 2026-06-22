import { requestClient } from '#/api/request';

export interface Article {
  audioError?: string;
  audioStatus?: 'failed' | 'generating' | 'none' | 'ready' | string;
  audioUrl?: string;
  author: string;
  category: string;
  content: string;
  cover: string;
  createTime: string;
  id: string;
  publishTime: string;
  sort: number;
  status: 'draft' | 'published' | string;
  summary: string;
  tags: string[];
  title: string;
  updateTime: string;
  viewCount: number;
  voiceKey?: string;
}

export interface ArticleInput {
  author?: string;
  category?: string;
  content: string;
  cover?: string;
  id?: string;
  sort?: number;
  status?: 'draft' | 'published' | string;
  summary?: string;
  tags?: string[];
  title: string;
  voiceKey?: string;
}

interface ArticlePageResult<T> {
  items: T[];
  total: number;
}

export function getArticlesApi(params?: Record<string, any>) {
  return requestClient.get<ArticlePageResult<Article>>('/articles', { params });
}

export function getArticleApi(id: string) {
  return requestClient.get<Article>(`/articles/${id}`);
}

export function createArticleApi(data: ArticleInput) {
  return requestClient.post<Article>('/articles', data);
}

export function updateArticleApi(id: string, data: ArticleInput) {
  return requestClient.put<Article>(`/articles/${id}`, data);
}

export function deleteArticleApi(id: string) {
  return requestClient.delete<boolean>(`/articles/${id}`);
}

// 触发听书音频生成（长文分片串行合成，耗时较长）。
export function generateArticleAudioApi(id: string) {
  return requestClient.post<Article>(`/articles/${id}/audio`, undefined, {
    timeout: 600_000,
  });
}

// 全局默认听书音色读写。
export function getReadingSettingsApi() {
  return requestClient.get<{ voiceKey: string }>('/reading/settings');
}

export function updateReadingSettingsApi(voiceKey: string) {
  return requestClient.put<{ voiceKey: string }>('/reading/settings', {
    voiceKey,
  });
}
