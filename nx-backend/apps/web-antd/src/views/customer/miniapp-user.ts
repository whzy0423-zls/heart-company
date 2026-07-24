type QueryValue = null | string | string[] | undefined;
type Query = Record<string, QueryValue>;

function positiveID(value: QueryValue) {
  const text = Array.isArray(value) ? value[0] : value;
  return typeof text === 'string' && /^[1-9][0-9]*$/.test(text) ? text : undefined;
}

export function miniappOpenIntent(query: Query) {
  const userId = positiveID(query.userId);
  if (!userId) return undefined;
  const open = Array.isArray(query.open) ? query.open[0] : query.open;
  if (open === 'detail') return { mode: 'detail' as const, userId };
  if (open === 'test') {
    const testRecordId = positiveID(query.testRecordId);
    if (testRecordId) return { mode: 'test' as const, testRecordId, userId };
  }
  return undefined;
}

export function bookingSignupTarget(signupId: string) {
  const leadId = positiveID(signupId);
  if (!leadId) return undefined;
  return { path: '/customer/signups', query: { leadId, open: 'detail' } };
}
