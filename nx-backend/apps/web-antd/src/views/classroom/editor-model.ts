import type {
  ClassroomAccessLevel,
  ClassroomContent,
  ClassroomContentCreatePayload,
  ClassroomSeries,
} from '#/api/core/classroom';

export function createContentDraftDefaults(): ClassroomContentCreatePayload & {
  accessLevel: ClassroomAccessLevel;
  priceCents: number;
} {
  return {
    accessLevel: 'public',
    contentType: 'video',
    priceCents: 0,
    showAsStandalone: false,
    title: '',
  };
}

export function contentMetadataPayload(
  form: ClassroomContentCreatePayload,
): ClassroomContentCreatePayload {
  const {
    badge,
    contentType,
    coverUrl,
    description,
    durationSeconds,
    episodeNo,
    recordedAt,
    seriesId,
    showAsStandalone,
    sortOrder,
    tags,
    teacherKey,
    teacherName,
    title,
  } = form;
  return {
    badge,
    contentType,
    coverUrl,
    description,
    durationSeconds,
    episodeNo,
    recordedAt,
    seriesId,
    showAsStandalone,
    sortOrder,
    tags,
    teacherKey,
    teacherName,
    title,
  };
}

export function purchaseStrategyRequired(
  form: Pick<ClassroomContent, 'showAsStandalone'> & {
    accessLevel: ClassroomAccessLevel;
    seriesId?: number;
  },
  series?: ClassroomSeries,
) {
  return Boolean(
    form.showAsStandalone &&
    form.seriesId &&
    form.accessLevel === 'inherit' &&
    series?.accessLevel === 'paid',
  );
}
