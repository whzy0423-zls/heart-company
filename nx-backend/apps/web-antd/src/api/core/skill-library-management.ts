import { requestClient } from '#/api/request';

export type SkillLibraryAdminStatus = 'disabled' | 'enabled';

export interface SkillLibraryAdminLibrary {
  description: string;
  iconKey: string;
  id: number;
  key: string;
  name: string;
  sortOrder: number;
  status: SkillLibraryAdminStatus;
}

export interface SkillLibraryAdminCategory {
  colorToken: string;
  iconKey: string;
  id: number;
  key: string;
  libraryId: number;
  name: string;
  skillCount: number;
  sortOrder: number;
  status: SkillLibraryAdminStatus;
}

export interface SkillLibraryAdminSkill {
  categoryId: number;
  categoryKey: string;
  categoryName: string;
  colorToken: string;
  description: string;
  hasPublishedVersion: boolean;
  iconKey: string;
  id: number;
  key: string;
  name: string;
  publishedVersion?: string;
  sortOrder: number;
  status: SkillLibraryAdminStatus;
  summary: string;
  updatedAt?: string;
}

export interface SkillLibraryManagementCatalog {
  categories: SkillLibraryAdminCategory[];
  libraries: SkillLibraryAdminLibrary[];
  skills: SkillLibraryAdminSkill[];
}

export interface SkillLibraryMetadataInput {
  colorToken: string;
  description: string;
  iconKey: string;
  name: string;
  sortOrder: number;
  status: SkillLibraryAdminStatus;
}

export interface SkillLibrarySkillInput extends SkillLibraryMetadataInput {
  categoryId: number;
  summary: string;
}

export function getSkillLibraryManagementApi() {
  return requestClient.get<SkillLibraryManagementCatalog>('/skill-library-management');
}

export function updateSkillLibraryApi(id: number, data: SkillLibraryMetadataInput) {
  return requestClient.request<{ id: number; updated: boolean }>(`/skill-library-management/library/${id}`, {
    data,
    method: 'PATCH',
  });
}

export function updateSkillLibraryCategoryApi(id: number, data: SkillLibraryMetadataInput) {
  return requestClient.request<{ id: number; updated: boolean }>(`/skill-library-management/categories/${id}`, {
    data,
    method: 'PATCH',
  });
}

export function updateSkillLibrarySkillApi(id: number, data: SkillLibrarySkillInput) {
  return requestClient.request<{ id: number; updated: boolean }>(`/skill-library-management/skills/${id}`, {
    data,
    method: 'PATCH',
  });
}
