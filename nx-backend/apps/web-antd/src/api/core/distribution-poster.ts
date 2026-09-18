import { requestClient } from '#/api/request';

export interface PosterTemplate {
  id: string;
  name: string;
  enabled: boolean;
  sortOrder: number;
  templateUrl: string;
  headline?: string;
  subtitle?: string;
  cta?: string;
  landingUrl: string;
  qrImageUrl: string;
  qrSize: number;
  qrX: number;
  qrY: number;
  inviteX: number;
  inviteY: number;
  inviteWidth: number;
  inviteFontSize: number;
}

export interface PosterConfig extends PosterTemplate {
  templates: PosterTemplate[];
}

export const getPosterConfigApi = () => requestClient.get<PosterConfig>('/distribution-poster-config');
export const savePosterConfigApi = (data: PosterConfig) => requestClient.put<PosterConfig>('/distribution-poster-config', data);
