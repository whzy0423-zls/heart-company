# 后台管理前端（web-antd）镜像
# 构建上下文为仓库根 nine-xing/（compose 中 context: .）
# ---- build stage ----
FROM node:22-slim AS builder

ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
ENV NODE_OPTIONS=--max-old-space-size=8192
ENV TZ=Asia/Shanghai
ENV CI=true
# 国内网络：corepack 下载 pnpm、pnpm 装依赖都走 npmmirror 镜像
ENV COREPACK_NPM_REGISTRY=https://registry.npmmirror.com
ENV npm_config_registry=https://registry.npmmirror.com

RUN npm i -g corepack && corepack enable

WORKDIR /app

# 拷贝整个 monorepo（web-antd 依赖 packages/ internal/ 才能构建）
COPY nx-backend/ /app/

RUN pnpm install --frozen-lockfile
RUN pnpm run build:antd

# ---- runtime stage ----
FROM nginx:stable-alpine AS production

RUN echo "types { application/javascript js mjs; }" > /etc/nginx/conf.d/mjs.conf \
    && rm -rf /etc/nginx/conf.d/default.conf

COPY --from=builder /app/apps/web-antd/dist /usr/share/nginx/html
COPY nx-backend/scripts/deploy/nginx.conf /etc/nginx/nginx.conf

EXPOSE 8080
CMD ["nginx", "-g", "daemon off;"]
