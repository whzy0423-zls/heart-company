# 官网（website-react）镜像
# 构建上下文为仓库根 nine-xing/（compose 中 context: .），
# 因为 website-react 构建时会 import ../shared/site-config.json
# ---- build stage ----
FROM node:22-slim AS builder

ENV TZ=Asia/Shanghai
ENV CI=true
# 国内网络：依赖安装走 npmmirror 镜像
ENV npm_config_registry=https://registry.npmmirror.com
WORKDIR /app

# 先拷依赖清单装依赖（利用缓存）
COPY website-react/package.json website-react/package-lock.json* /app/website-react/
WORKDIR /app/website-react
RUN npm ci

# 再拷源码 + 共享配置
COPY website-react/ /app/website-react/
COPY shared/ /app/shared/

RUN npm run build

# ---- runtime stage ----
FROM nginx:stable-alpine AS production

RUN echo "types { application/javascript js mjs; }" > /etc/nginx/conf.d/mjs.conf \
    && rm -rf /etc/nginx/conf.d/default.conf

COPY --from=builder /app/website-react/dist /usr/share/nginx/html
COPY website-react/nginx.conf /etc/nginx/nginx.conf

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
