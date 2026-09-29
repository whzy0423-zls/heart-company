export interface TeacherEnrollmentSource {
  interest?: string | null;
  message?: string | null;
  sourcePlatform?: string | null;
}

export interface TeacherEnrollmentDetails {
  isTeacherEnrollment: boolean;
  teacherKey: string;
  teacherName: string;
  kind: string;
  intent: string;
  preferredTime: string;
  message: string;
}

const EMPTY_DETAILS: TeacherEnrollmentDetails = {
  isTeacherEnrollment: false,
  teacherKey: '',
  teacherName: '',
  kind: '',
  intent: '',
  preferredTime: '',
  message: '',
};

const FIELD_ALIASES = {
  intent: '意向方向',
  kind: '报名类型',
  message: '留言',
  preferredTime: '期望时间',
  teacher: '老师',
} as const;

type EnrollmentField = keyof typeof FIELD_ALIASES;

const FIELD_PATTERN =
  /(?:^|\n)\s*(老师|报名老师|报名类型|类型|意向方向|意向|期望时间|留言)\s*[:：]\s*([^\n]*)/g;

/**
 * Reads the stable, human-readable encoding used by the app teacher enrollment
 * endpoint while leaving ordinary website/miniapp leads untouched.
 */
export function parseTeacherEnrollment(
  source: TeacherEnrollmentSource,
): TeacherEnrollmentDetails {
  const interest = normalizeFieldSeparators(source.interest || '');
  const message = normalizeFieldSeparators(source.message || '');
  const fields = new Map<EnrollmentField, string>();

  for (const value of [interest, message]) {
    for (const match of value.matchAll(FIELD_PATTERN)) {
      const field = fieldAlias(match[1] ?? '');
      if (field && !fields.has(field)) {
        fields.set(field, (match[2] ?? '').trim());
      }
    }
  }

  const teacher = parseTeacherValue(fields.get('teacher') || '');
  const kind = fields.get('kind') || '';
  const intent = fields.get('intent') || '';
  const preferredTime = fields.get('preferredTime') || '';
  const enrollmentMessage = fields.get('message') || '';
  // A teacher enrollment always carries the teacher marker and its kind. This
  // pair keeps ordinary legacy messages containing a lone “期望时间” label
  // from being reclassified in the management table.
  const isTeacherEnrollment = Boolean((teacher.key || teacher.name) && kind);

  if (!isTeacherEnrollment) {
    return { ...EMPTY_DETAILS };
  }

  return {
    isTeacherEnrollment: true,
    teacherKey: teacher.key,
    teacherName: teacher.name,
    kind,
    intent,
    preferredTime,
    message: enrollmentMessage,
  };
}

export function enrollmentKindLabel(value?: string | null): string {
  const normalized = (value || '').trim();
  return (
    {
      consult: '1v1 咨询预约',
      course: '课程报名',
      enterprise: '企业课程咨询',
    }[normalized] ||
    normalized ||
    '-'
  );
}

function normalizeFieldSeparators(value: string): string {
  return value
    .replace(/\r\n?/g, '\n')
    .replace(/\s*[|｜]\s*/g, '\n')
    .trim();
}

function fieldAlias(value: string): EnrollmentField | undefined {
  switch (value.trim()) {
    case '老师':
    case '报名老师':
      return 'teacher';
    case '报名类型':
    case '类型':
      return 'kind';
    case '意向方向':
    case '意向':
      return 'intent';
    case '期望时间':
      return 'preferredTime';
    case '留言':
      return 'message';
    default:
      return undefined;
  }
}

function parseTeacherValue(value: string): { key: string; name: string } {
  const match = value.match(/^(.*?)\s*[（(]([^）)]+)[）)]\s*$/);
  if (!match) return { key: '', name: value.trim() };
  return { key: (match[2] ?? '').trim(), name: (match[1] ?? '').trim() };
}
