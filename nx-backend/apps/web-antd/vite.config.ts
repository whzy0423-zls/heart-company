import { defineConfig } from '@vben/vite-config';

export default defineConfig(async () => {
  return {
    application: {},
    vite: {
      plugins: [
        {
          name: 'infinite-canvas-dev-entry',
          configureServer(server) {
            server.middlewares.use((req, res, next) => {
              const pathname = req.url?.split('?')[0];
              if (
                pathname === '/infinite-canvas' ||
                pathname === '/infinite-canvas/'
              ) {
                res.statusCode = 302;
                res.setHeader('Location', '/infinite-canvas/index.html#/canvas');
                res.end();
                return;
              }
              next();
            });
          },
        },
      ],
      server: {
        proxy: {
          '/api': {
            changeOrigin: true,
            rewrite: (path) => path.replace(/^\/api/, ''),
            // mock代理目标地址
            target: 'http://localhost:5320/api',
            ws: true,
          },
        },
      },
    },
  };
});
