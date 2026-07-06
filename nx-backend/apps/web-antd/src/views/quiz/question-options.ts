import type { QuizOption } from '#/api';

function normalizeWeights(weights: unknown): Record<string, number> {
  if (!weights || typeof weights !== 'object' || Array.isArray(weights)) {
    return {};
  }

  return Object.entries(weights as Record<string, unknown>).reduce<Record<string, number>>(
    (result, [key, value]) => {
      const type = Number(key);
      const score = Number(value);
      if (Number.isInteger(type) && type >= 1 && type <= 9 && Number.isFinite(score)) {
        result[String(type)] = score;
      }
      return result;
    },
    {},
  );
}

export function parseAndValidateQuizOptions(text: string, status: string): QuizOption[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text || '[]');
  } catch {
    throw new Error('选项 JSON 格式不正确');
  }

  if (!Array.isArray(parsed)) {
    throw new Error('选项 JSON 格式不正确');
  }

  const options = parsed.map((item) => ({
    id: String((item as any)?.id || '').trim(),
    text: String((item as any)?.text || '').trim(),
    weights: normalizeWeights((item as any)?.weights),
  }));

  if (options.length < 2 || options.some((item) => !item.id || !item.text)) {
    throw new Error('请至少填写两个完整选项，并确保每个选项包含 id 和 text');
  }

  const optionIds = new Set<string>();
  if (options.some((item) => optionIds.size === optionIds.add(item.id).size)) {
    throw new Error('选项 id 不能重复');
  }

  if (status === 'enabled' && options.some((item) => Object.keys(item.weights || {}).length === 0)) {
    throw new Error('启用题目的每个选项都必须配置有效 weights');
  }

  return options;
}
