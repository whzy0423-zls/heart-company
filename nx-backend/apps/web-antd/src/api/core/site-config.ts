import { requestClient } from '#/api/request';

export interface MiniappCarouselItem {
  enabled: boolean;
  image: string;
}

export interface MiniappCarouselConfig {
  autoplay: boolean;
  interval: number;
  items: MiniappCarouselItem[];
}

export interface SiteConfig {
  home: {
    miniappCarousel?: MiniappCarouselConfig;
  } & Record<string, any>;
  navigation: {
    drawer: NavItem[];
    main: NavItem[];
    tabs: Array<NavItem & { icon: string; match: string }>;
  };
  site: {
    brandName: string;
    copyright: string;
    footerTagline: string;
    logo: string;
    customerServiceQr: string;
  };
  types: EnneagramType[];
}

export interface NavItem {
  label: string;
  to: string;
  type: string;
}

export interface EnneagramType {
  avatar: string;
  description: string;
  id: string;
  keywords: string;
  name: string;
}

export function getSiteConfigApi() {
  return requestClient.get<SiteConfig>('/site-config');
}

export function updateSiteConfigApi(data: SiteConfig) {
  return requestClient.put<SiteConfig>('/site-config', data);
}

export interface SiteBuildStatus {
  durationMs: number;
  finishedAt: string;
  log: string;
  message: string;
  queuedNext: boolean;
  startedAt: string;
  state: string;
}

export function getSiteBuildStatusApi() {
  return requestClient.get<SiteBuildStatus>('/site-config/build-status');
}
