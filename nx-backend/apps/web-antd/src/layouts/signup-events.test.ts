import { afterEach, describe, expect, it } from 'vitest';

import {
  buildSignupEventsURL,
  extractSignupStreamEvents,
  signupNoticeIdentity,
  shouldLogoutForSignupEventStatus,
  shouldPollSignupNoticeFallback,
} from './signup-events';

const originalReplaceAll = String.prototype.replaceAll;

describe('signup event stream parser', () => {
  afterEach(() => {
    Object.defineProperty(String.prototype, 'replaceAll', {
      configurable: true,
      value: originalReplaceAll,
      writable: true,
    });
  });

  it('extracts complete events and keeps partial tail', () => {
    const result = extractSignupStreamEvents(
      ': connected\n\n' +
        'event: signup\n' +
        'data: {"id":1}\n\n' +
        'event: signup\n' +
        'data: {"id":',
    );

    expect(result.events).toEqual([
      {
        data: '{"id":1}',
        event: 'signup',
      },
    ]);
    expect(result.remaining).toBe('event: signup\ndata: {"id":');
  });

  it('joins multi-line data fields', () => {
    const result = extractSignupStreamEvents(
      'event: signup\n' + 'data: {"id":1,\n' + 'data: "name":"a"}\n\n',
    );

    expect(result.events).toEqual([
      {
        data: '{"id":1,\n"name":"a"}',
        event: 'signup',
      },
    ]);
    expect(result.remaining).toBe('');
  });

  it('parses CRLF streams when replaceAll is unavailable', () => {
    Object.defineProperty(String.prototype, 'replaceAll', {
      configurable: true,
      value: undefined,
      writable: true,
    });

    const result = extractSignupStreamEvents(
      'event: signup\r\n' + 'data: {"id":1}\r\n\r\n',
    );

    expect(result.events).toEqual([
      {
        data: '{"id":1}',
        event: 'signup',
      },
    ]);
    expect(result.remaining).toBe('');
  });
});

describe('signup event stream url', () => {
  it('builds signup event url from configured api base', () => {
    expect(buildSignupEventsURL('/api')).toBe('/api/signups/events');
    expect(buildSignupEventsURL('/api/')).toBe('/api/signups/events');
    expect(buildSignupEventsURL('https://example.com/app-api')).toBe(
      'https://example.com/app-api/signups/events',
    );
  });

  it('falls back to same-origin api prefix when api base is empty', () => {
    expect(buildSignupEventsURL('')).toBe('/api/signups/events');
  });
});

describe('signup notice identity', () => {
  it('separates anonymous, authenticated, and named users', () => {
    expect(signupNoticeIdentity({ accessToken: null, username: 'admin' })).toBe(
      'anonymous',
    );
    expect(signupNoticeIdentity({ accessToken: 'token' })).toBe(
      'authenticated',
    );
    expect(
      signupNoticeIdentity({ accessToken: 'token', userId: 7 }),
    ).toBe('7');
    expect(
      signupNoticeIdentity({ accessToken: 'token', username: 'admin' }),
    ).toBe('admin');
  });
});

describe('signup event stream fallback policy', () => {
  it('logs out on unauthorized stream responses', () => {
    expect(shouldLogoutForSignupEventStatus(401)).toBe(true);
    expect(shouldLogoutForSignupEventStatus(403)).toBe(true);
    expect(shouldLogoutForSignupEventStatus(500)).toBe(false);
  });

  it('polls only when stream is unavailable or not connected', () => {
    expect(
      shouldPollSignupNoticeFallback({
        connecting: false,
        controllerActive: true,
        unavailable: false,
      }),
    ).toBe(false);
    expect(
      shouldPollSignupNoticeFallback({
        connecting: false,
        controllerActive: false,
        unavailable: true,
      }),
    ).toBe(true);
    expect(
      shouldPollSignupNoticeFallback({
        connecting: false,
        controllerActive: false,
        unavailable: false,
      }),
    ).toBe(true);
  });
});
