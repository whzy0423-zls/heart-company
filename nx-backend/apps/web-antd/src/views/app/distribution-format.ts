// All distribution API money fields are integer cents.
export function formatYuan(value: number | string | null | undefined) {
  const yuan = Number(value ?? 0) / 100;
  if (!Number.isFinite(yuan) || yuan === 0) return '¥0';
  return `¥${yuan.toLocaleString('zh-CN', {
    maximumFractionDigits: 2,
    minimumFractionDigits: Number.isInteger(yuan) ? 0 : 2,
  })}`;
}

export type SettlementAction = 'approve' | 'cancel' | 'paid' | 'reject';
export function settlementActionPayload(action: SettlementAction, input: string) {
  const text = input.trim();
  if (action === 'paid') {
    if (!text || new TextEncoder().encode(text).length > 200) throw new Error('请填写打款凭证号（最多 200 字节）');
    return { paymentReference: text };
  }
  if (action === 'reject' || action === 'cancel') {
    if (!text) throw new Error('请填写操作原因');
    return { reason: text };
  }
  return {};
}
