FROM node:20-alpine AS builder

WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci

COPY . .
RUN NODE_ENV=production npm run build

FROM nginx:1.25-alpine
# 把 build 结果放到 /usr/share/nginx/html/admin/
# 以便 main nginx 可以 alias 到这里，或直接用本服务的 /admin/ location
COPY --from=builder /app/dist /usr/share/nginx/html/admin
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
