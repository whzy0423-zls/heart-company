import type { PageResult } from './signup';

import { requestClient } from '#/api/request';

/** 资产类型：场景 / 人物 / 音频 / 视频 */
export type VideoAssetType =
  | 'audio'
  | 'character'
  | 'outfit'
  | 'prop'
  | 'scene'
  | 'style'
  | 'video';

export interface VideoAsset {
  assetId: string;
  coverUrl: string;
  createTime: string;
  id: string;
  name: string;
  remark: string;
  status: string;
  type: VideoAssetType;
  updateTime: string;
  url: string;
}

export function listAssetsApi(params?: {
  keyword?: string;
  page?: number;
  pageSize?: number;
  type?: VideoAssetType;
}) {
  return requestClient.get<PageResult<VideoAsset>>('/video/assets/list', {
    params,
  });
}

export function createAssetApi(data: {
  assetId?: string;
  coverUrl?: string;
  name: string;
  remark?: string;
  type?: VideoAssetType;
  url: string;
}) {
  return requestClient.post<VideoAsset>('/video/assets', data);
}

export function deleteAssetApi(id: string) {
  return requestClient.delete<boolean>(`/video/assets/${id}`);
}

/** 文生图：调用 gpt-image-2 网关生成图片并登记为资产 */
export function generateImageAssetApi(data: {
  model?: string;
  name?: string;
  prompt: string;
  remark?: string;
  size?: string;
  type?: VideoAssetType;
}) {
  return requestClient.post<VideoAsset>('/video/assets/generate-image', data, {
    timeout: 180_000,
  });
}

/** 一键润色：复用对话模型把方向/草稿润色成高质量的文生图/文生视频提示词 */
export function polishPromptApi(data: {
  kind: 'image' | 'video';
  prompt: string;
}) {
  return requestClient.post<{ prompt: string }>(
    '/video/assets/polish-prompt',
    data,
    { timeout: 60_000 },
  );
}
