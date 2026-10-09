import { requestClient } from '#/api/request';

export interface AppUserReportClaim {
  evidenceRefs: string[];
  text: string;
}

export interface AppUserReportSummary {
  evidenceCount: number;
  generatedAt: string;
  id: number;
  sourceThrough: string;
  version: number;
}

export interface AppUserReportList {
  enabled: boolean;
  reports: AppUserReportSummary[];
  status:
    | 'analyzing'
    | 'disabled'
    | 'failed'
    | 'insufficient_data'
    | 'pending'
    | 'ready';
}

export interface AppUserReport extends AppUserReportSummary {
  analysis: {
    actions?: Array<{
      detail: string;
      evidenceRefs: string[];
      title: string;
    }>;
    awarenessPrompts?: AppUserReportClaim[];
    changes: AppUserReportClaim[];
    goals: AppUserReportClaim[];
    observations: AppUserReportClaim[];
    patterns: AppUserReportClaim[];
    recommendations: AppUserReportClaim[];
    strengths?: AppUserReportClaim[];
    stressPoints?: AppUserReportClaim[];
    summary: AppUserReportClaim;
    trendExplanation?: AppUserReportClaim;
    uncertainties: AppUserReportClaim[];
    weeklyReview?: AppUserReportClaim;
  };
  appUserId: number;
  cardId: number;
  coverage: {
    eligibleCount: number;
    includedCount: number;
    omittedCount: number;
    truncated: boolean;
    truncatedCount: number;
    windowDays: number;
  };
  evidence: Array<{
    id: string;
    kind: string;
    occurredAt: string;
    text: string;
  }>;
  periodEnd: string;
  periodStart: string;
  published: boolean;
  sourceFrom: string;
  timezone: string;
}

export function getAppUserReportsApi(appUserId: number) {
  return requestClient.get<AppUserReportList>('/app-user-reports', {
    params: { appUserId },
  });
}

export function getAppUserReportApi(id: number) {
  return requestClient.get<AppUserReport>(`/app-user-reports/${id}`);
}

export function requestAppUserReportApi(appUserId: number) {
  return requestClient.post<{ queued: boolean; reason?: string }>(
    '/app-user-reports',
    {
      appUserId,
    },
  );
}
