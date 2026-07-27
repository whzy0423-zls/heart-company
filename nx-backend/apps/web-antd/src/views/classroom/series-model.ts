import type {
  ClassroomSeries,
  ClassroomSeriesCreatePayload,
} from '#/api/core/classroom';

export function seriesMetadataPayload(form: {
  coverAssetId?: number;
  coverUrl?: string;
  sortOrder?: number;
  summary?: string;
  teacherKey?: string;
  teacherName?: string;
  title: string;
}): ClassroomSeriesCreatePayload {
  return {
    coverAssetId: form.coverAssetId,
    coverUrl: form.coverUrl,
    sortOrder: form.sortOrder,
    summary: form.summary,
    teacherKey: form.teacherKey,
    teacherName: form.teacherName,
    title: form.title,
  };
}

export function seriesPricePayload(
  series: ClassroomSeries,
  accessLevel: ClassroomSeries['accessLevel'],
  priceCents: number,
) {
  return { accessLevel, expectedUpdatedAt: series.updatedAt, priceCents };
}
