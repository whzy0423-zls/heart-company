import { describe, expect, it, vi } from 'vitest';

vi.mock('#/layouts', () => ({ BasicLayout: { name: 'BasicLayout' } }));

import routes from './routes/modules/video';

describe('video generation routes', () => {
  it('exposes only the Infinite Canvas page below the video menu', () => {
    expect(routes).toHaveLength(1);

    const videoRoute = routes[0];
    expect(videoRoute?.path).toBe('/video');
    expect(videoRoute?.meta?.title).toBe('视频生成');
    expect(videoRoute?.children).toHaveLength(1);
    expect(videoRoute?.children?.[0]).toMatchObject({
      name: 'InfiniteCanvas',
      path: 'infinite-canvas',
      meta: {
        icon: 'lucide:workflow',
        title: '无限画布',
      },
    });
  });
});
