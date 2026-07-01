import { requestClient } from '#/api/request';

export interface UploadedFile {
  assetId?: number;
  assetKey?: string;
  contentType: string;
  key: string;
  name: string;
  objectKey?: string;
  objectUrl?: string;
  size: number;
  url: string;
}

export function uploadFileApi(file: File, dir = 'site') {
  return requestClient.upload<UploadedFile>(
    '/upload',
    { file },
    {
      params: { dir },
      timeout: 120_000,
    },
  );
}
