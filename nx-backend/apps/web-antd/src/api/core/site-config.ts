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

export type MiniappHomeEntryKey = 'learn' | 'profile' | 'relation' | 'test';

export type MiniappHomeIconKey =
  | 'book'
  | 'compass'
  | 'growth'
  | 'heart'
  | 'relation'
  | 'spark';

export type MiniappHomeThemeKey =
  | 'blue'
  | 'cyan'
  | 'orange'
  | 'pink'
  | 'purple';

export interface MiniappHomeSectionBase {
  enabled: boolean;
}

export interface MiniappHomeBrand extends MiniappHomeSectionBase {
  name: string;
  tagline: string;
}

export interface MiniappHomeHero extends MiniappHomeSectionBase {
  buttonText: string;
  description: string;
  kicker: string;
  title: string;
}

export interface MiniappHomeEntry extends MiniappHomeSectionBase {
  description: string;
  icon: MiniappHomeIconKey;
  key: MiniappHomeEntryKey;
  theme: MiniappHomeThemeKey;
  title: string;
}

export interface MiniappHomeEntriesSection extends MiniappHomeSectionBase {
  description: string;
  items: MiniappHomeEntry[];
  title: string;
}

export interface MiniappHomeGrowth extends MiniappHomeSectionBase {
  description: string;
  eyebrow: string;
  title: string;
}

export interface MiniappHomeConfig {
  brand: MiniappHomeBrand;
  entriesSection: MiniappHomeEntriesSection;
  growth: MiniappHomeGrowth;
  hero: MiniappHomeHero;
}

export interface MiniappLearnHero {
  eyebrow: string;
  lead: string;
  meta: string[];
  title: string;
}

export interface MiniappLearnClassroom {
  ctaText: string;
  emptyActionText: string;
  emptyDescription: string;
  emptyTitle: string;
  eyebrow: string;
  heroEyebrow: string;
  heroLead: string;
  heroTitle: string;
  moreText: string;
  title: string;
}

export interface MiniappLearnSection {
  eyebrow: string;
  title: string;
}

export interface MiniappLearnCoursesSection extends MiniappLearnSection {
  emptyDescription: string;
  emptyTitle: string;
}

export interface MiniappLearnQuotesSection extends MiniappLearnSection {
  emptyTitle: string;
}

export interface MiniappLearnSections {
  courses: MiniappLearnCoursesSection;
  quotes: MiniappLearnQuotesSection;
  teacher: MiniappLearnSection;
  types: MiniappLearnSection;
}

export interface MiniappLearnConfig {
  bottomCtaText: string;
  classroom: MiniappLearnClassroom;
  hero: MiniappLearnHero;
  sections: MiniappLearnSections;
}

export interface EnterpriseServiceItem {
  description: string;
  title: string;
}

export interface EnterpriseProcessStep {
  description: string;
  title: string;
}

export interface EnterpriseConfig {
  buttonHref: string;
  buttonText: string;
  eyebrow: string;
  items: EnterpriseServiceItem[];
  lead: string;
  moduleTitle: string;
  modules: string[];
  processSteps: EnterpriseProcessStep[];
  title: string;
}

export interface SiteConfig {
  home: {
    enterprise?: EnterpriseConfig;
    miniappCarousel?: MiniappCarouselConfig;
    miniappHome?: MiniappHomeConfig;
    miniappLearn?: MiniappLearnConfig;
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
