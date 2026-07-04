export interface InsightStatusSource {
  memoryCount?: number;
  primaryType?: number;
}

export function enneagramLabel(type?: number) {
  return type && type > 0 ? `${type}号` : '-';
}

export function getProfileSummary(profile?: unknown) {
  if (!profile || typeof profile !== 'object') {
    return '暂无画像摘要';
  }
  const data = profile as Record<string, unknown>;
  const summary = typeof data.summary === 'string' ? data.summary.trim() : '';
  if (summary) return summary;
  const title = typeof data.title === 'string' ? data.title.trim() : '';
  return title || '暂无画像摘要';
}

export function getProfileList(profile: unknown, key: string): string[] {
  if (!profile || typeof profile !== 'object') return [];
  const value = (profile as Record<string, unknown>)[key];
  if (!Array.isArray(value)) return [];
  return value.filter((item): item is string => typeof item === 'string');
}

export function getCenterSummary(centers?: unknown) {
  const items = Array.isArray(centers)
    ? centers
    : centers && typeof centers === 'object'
      ? Object.values(centers)
      : [];

  const summary = items
    .map((item) => {
      if (!item || typeof item !== 'object') return '';
      const data = item as Record<string, unknown>;
      const name = typeof data.name === 'string' ? data.name.trim() : '';
      const rawPct = data.pct ?? data.percent ?? data.value;
      const pct =
        typeof rawPct === 'string' ? Number.parseFloat(rawPct) : Number(rawPct);
      if (!name || !Number.isFinite(pct)) return '';
      return `${name} ${Math.round(pct)}%`;
    })
    .filter(Boolean);

  return summary.length > 0 ? summary.join(' / ') : '-';
}

export function getScoreTags(score?: unknown) {
  if (!score || typeof score !== 'object') return [];
  return Object.entries(score as Record<string, unknown>)
    .map(([type, value]) => ({
      score: typeof value === 'string' ? Number(value) : Number(value),
      type: Number(type),
    }))
    .filter(({ score, type }) => Number.isFinite(type) && Number.isFinite(score))
    .sort((left, right) => left.type - right.type)
    .map(({ score, type }) => `${type}号 ${score}分`);
}

export function getUserInsightStatus(source: InsightStatusSource) {
  if ((source.memoryCount ?? 0) > 0) return '已有沉淀';
  if ((source.primaryType ?? 0) > 0) return '已有画像';
  return '待沉淀';
}
