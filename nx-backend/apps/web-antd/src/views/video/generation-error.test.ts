import { describe, expect, it } from 'vitest';

import { formatGenerationError } from './generation-error';

describe('generation error helpers', () => {
  it('formats image url 403 gateway json into readable text', () => {
    expect(
      formatGenerationError(
        '{"error":{"message":"invalid image_url: image url returned 403","type":"invalid_request_error"}}',
      ),
    ).toContain('参考图片地址无法被视频网关访问');
  });

  it('unwraps nested json message strings', () => {
    expect(
      formatGenerationError(
        String.raw`{"message":"{\"error\":{\"message\":\"invalid video_url: video url returned 403\"}}"}`,
      ),
    ).toContain('参考视频地址无法被视频网关访问');
  });
});
