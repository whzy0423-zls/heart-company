export function memberPlanLabel(level?: string) {
  const labels: Record<string, string> = {
    free: '普通用户',
    svip: '年包会员',
    vip: '月包会员',
    vip_month: '月包会员',
    vip_quarter: '季包会员',
    vip_year: '年包会员',
  };
  return labels[level || ''] || level || '-';
}

export function membershipStatusLabel(status?: string) {
  const labels: Record<string, string> = {
    closed: '已关闭',
    paid: '已开通',
    pending: '待客服确认',
    pending_confirmation: '待客服确认',
  };
  return labels[status || ''] || status || '-';
}

export function buildMembershipGrantPayload(activationAt: Date) {
  return { activationAt: activationAt.toISOString() };
}

export function previewMembershipExpiry(
  currentExpiresAt: string | undefined,
  activationAt: Date,
  durationDays: number,
) {
  if (!Number.isFinite(durationDays) || durationDays <= 0) return undefined;
  const normalizedCurrent = currentExpiresAt?.replaceAll('/', '-');
  const currentExpiry = normalizedCurrent
    ? new Date(normalizedCurrent)
    : undefined;
  const base =
    currentExpiry &&
    !Number.isNaN(currentExpiry.getTime()) &&
    currentExpiry > activationAt
      ? currentExpiry
      : activationAt;
  return new Date(base.getTime() + durationDays * 24 * 60 * 60 * 1000);
}
