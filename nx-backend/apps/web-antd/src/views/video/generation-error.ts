function unwrapGatewayMessage(raw: string, depth = 0): string {
  const text = raw.trim();
  if (!text) return '';
  if (depth >= 4) return text;

  try {
    const payload = JSON.parse(text) as unknown;
    const message = findGatewayMessage(payload, depth + 1);
    return message || text;
  } catch {
    return text;
  }
}

function findGatewayMessage(payload: unknown, depth: number): string {
  if (typeof payload === 'string') {
    return unwrapGatewayMessage(payload, depth);
  }
  if (!payload || typeof payload !== 'object') {
    return '';
  }
  const record = payload as Record<string, unknown>;
  for (const key of ['error', 'message', 'data']) {
    const message = findGatewayMessage(record[key], depth);
    if (message) return message;
  }
  return '';
}

export function formatGenerationError(raw?: string) {
  const message = unwrapGatewayMessage(raw || '');
  const lower = message.toLowerCase();
  if (lower.includes('invalid image_url') && lower.includes('returned 403')) {
    return '参考图片地址无法被视频网关访问（image_url 返回 403），请确认文件桶文件为公网可读、未开启防盗链/鉴权，或重新上传后使用 objectUrl';
  }
  if (lower.includes('invalid video_url') && lower.includes('returned 403')) {
    return '参考视频地址无法被视频网关访问（video_url 返回 403），请确认文件桶文件为公网可读、未开启防盗链/鉴权，或重新上传后使用 objectUrl';
  }
  if (lower.includes('invalid audio_url') && lower.includes('returned 403')) {
    return '参考音频地址无法被视频网关访问（audio_url 返回 403），请确认文件桶文件为公网可读、未开启防盗链/鉴权，或重新上传后使用 objectUrl';
  }
  return message || '-';
}
