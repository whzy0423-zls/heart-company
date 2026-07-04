interface SignupStreamEvent {
  data: string;
  event: string;
}

interface SignupNoticeFallbackState {
  connecting: boolean;
  controllerActive: boolean;
  unavailable: boolean;
}

export function buildSignupEventsURL(apiURL: string) {
  const base = (apiURL || '/api').replace(/\/+$/, '');
  return `${base}/signups/events`;
}

export function signupNoticeIdentity(input: {
  accessToken?: null | string;
  userId?: null | number | string;
  username?: null | string;
}) {
  if (!input.accessToken) {
    return 'anonymous';
  }
  return String(input.username || input.userId || 'authenticated');
}

export function shouldLogoutForSignupEventStatus(status: number) {
  return status === 401 || status === 403;
}

export function shouldPollSignupNoticeFallback(
  state: SignupNoticeFallbackState,
) {
  return state.unavailable || (!state.controllerActive && !state.connecting);
}

export function extractSignupStreamEvents(input: string): {
  events: SignupStreamEvent[];
  remaining: string;
} {
  const normalized = input.split('\r\n').join('\n');
  const blocks = normalized.split('\n\n');
  const remaining = blocks.pop() ?? '';
  const events = blocks
    .map(parseSignupStreamEventBlock)
    .filter((event): event is SignupStreamEvent => Boolean(event));

  return { events, remaining };
}

function parseSignupStreamEventBlock(block: string): null | SignupStreamEvent {
  let event = 'message';
  const data: string[] = [];

  for (const line of block.split('\n')) {
    if (!line || line.startsWith(':')) continue;

    if (line.startsWith('event:')) {
      event = line.slice('event:'.length).trim();
      continue;
    }

    if (line.startsWith('data:')) {
      data.push(line.slice('data:'.length).trimStart());
    }
  }

  if (data.length === 0) return null;
  return {
    data: data.join('\n'),
    event,
  };
}
