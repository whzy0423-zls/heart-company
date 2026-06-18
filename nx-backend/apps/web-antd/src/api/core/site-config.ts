import { requestClient } from '#/api/request';

export interface SiteConfig {
  home: Record<string, any>;
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
