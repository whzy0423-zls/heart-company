import { requestClient } from '#/api/request';

export interface AdminAuditLog {
  action: string;
  after: Record<string, any>;
  before: Record<string, any>;
  createTime: string;
  id: number;
  ip: string;
  operatorId: number;
  operatorName: string;
  summary: string;
  targetId: string;
  targetType: string;
  userAgent: string;
}

export interface AdminAuditLogPageResult<T> {
  items: T[];
  total: number;
}

export function getAdminAuditLogsApi(params?: Record<string, any>) {
  return requestClient.get<AdminAuditLogPageResult<AdminAuditLog>>(
    '/audit-logs/list',
    { params },
  );
}
