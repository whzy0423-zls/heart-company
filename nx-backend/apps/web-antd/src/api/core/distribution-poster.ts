import { requestClient } from '#/api/request';

export interface PosterConfig {
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

export const getPosterConfigApi = () => requestClient.get<PosterConfig>('/distribution-poster-config');
export const savePosterConfigApi = (data: PosterConfig) => requestClient.put<PosterConfig>('/distribution-poster-config', data);
