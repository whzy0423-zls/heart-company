import { requestClient } from '#/api/request';

export type StorySkillCategory = 'fairy_tale' | 'folk' | 'myth' | 'novel' | 'realistic';

export interface StorySkillAdminItem {
  category: StorySkillCategory;
  categoryId: number;
  categoryName: string;
  hasDraft: boolean;
  id: number;
  instructions?: string;
  key: string;
  name: string;
  publishedVersion?: string;
  status: 'draft' | 'enabled' | 'published' | 'review';
  summary: string;
  updatedAt?: string;
  version: string;
}

export interface StorySkillUploadInput {
  category: StorySkillCategory;
  file?: File;
  instructions?: string;
  key: string;
  name: string;
  summary: string;
  version: string;
}

export function getStorySkillsApi() {
  return requestClient.get<StorySkillAdminItem[]>('/story-skills');
}

export function getStorySkillApi(id: number) {
  return requestClient.get<StorySkillAdminItem>(`/story-skills/${id}`);
}

export function uploadStorySkillApi(input: StorySkillUploadInput) {
  const file = input.file ?? new File(
    [input.instructions ?? ''],
    'SKILL.md',
    { type: 'text/markdown' },
  );
  return requestClient.upload<StorySkillAdminItem>('/story-skills/upload', { ...input, file }, {
    timeout: 120_000,
  });
}

export function updateStorySkillApi(id: number, input: StorySkillUploadInput) {
  const formData = new FormData();
  Object.entries(input).forEach(([key, value]) => {
    if (value !== undefined) formData.append(key, value);
  });
  return requestClient.request<StorySkillAdminItem>(`/story-skills/${id}`, {
    data: formData,
    headers: { 'Content-Type': 'multipart/form-data' },
    method: 'PATCH',
    timeout: 120_000,
  });
}

export function deleteStorySkillApi(id: number) {
  return requestClient.delete<{ deleted: boolean; id: number }>(`/story-skills/${id}`);
}

export function publishStorySkillApi(id: number) {
  return requestClient.post<{ id: number; status: string }>(`/story-skills/${id}/publish`, {});
}
