import type { AppAgentDiscountRule } from '#/api';

export function discountedPriceCents(
  basePriceCents: number,
  rule: AppAgentDiscountRule,
) {
  if (!rule.enabled || basePriceCents <= 0) return basePriceCents;
  const reduction =
    rule.mode === 'percent_off'
      ? Math.round((basePriceCents * rule.value) / 10_000)
      : rule.value;
  return Math.max(1, basePriceCents - reduction);
}

export function validateDiscountRule(
  basePriceCents: number,
  rule: AppAgentDiscountRule,
): null | string {
  if (!rule.enabled) return null;
  if (!Number.isInteger(basePriceCents) || basePriceCents <= 0) {
    return '套餐原价无效';
  }
  if (!Number.isInteger(rule.value) || rule.value <= 0) {
    return '请输入大于 0 的优惠值';
  }
  if (rule.mode === 'percent_off' && rule.value >= 10_000) {
    return '折扣减免比例须低于 100%';
  }
  if (rule.mode === 'amount_off' && rule.value >= basePriceCents) {
    return '减免金额须低于套餐原价';
  }
  if (discountedPriceCents(basePriceCents, rule) <= 0) {
    return '优惠价须大于 0 元';
  }
  return null;
}
