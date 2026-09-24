type ErrorRecord = Record<string, unknown>;

function asRecord(value: unknown): ErrorRecord | undefined {
  return value && typeof value === 'object'
    ? (value as ErrorRecord)
    : undefined;
}

function firstText(...values: unknown[]) {
  return values.find(
    (value): value is string =>
      typeof value === 'string' && value.trim().length > 0,
  );
}

/** Extract the backend message before falling back to a generic HTTP message. */
export function getRequestErrorMessage(
  error: unknown,
  fallback = '请求失败，请稍后重试',
) {
  const root = asRecord(error);
  const response = asRecord(root?.response);
  const responseData = asRecord(response?.data);
  const data = asRecord(responseData?.data);
  const result = asRecord(responseData?.result);

  const message = firstText(
    responseData?.error,
    responseData?.message,
    responseData?.msg,
    data?.error,
    data?.message,
    data?.msg,
    result?.error,
    result?.message,
    result?.msg,
    root?.message,
  );

  if (message) return message;

  const code = responseData?.code;
  if (typeof code === 'number' || typeof code === 'string') {
    return `${fallback}（业务码：${code}）`;
  }
  return fallback;
}

/** The platform has both legacy `0` and newer `200` success envelopes. */
export function isSuccessfulResponseCode(code: unknown) {
  return code === 0 || code === '0' || code === 200 || code === '200';
}
