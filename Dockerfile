# ============================================================
# Stage 1: Build frontend
# ============================================================
FROM node:22-alpine AS frontend-builder

WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --ignore-scripts
COPY frontend/ ./
ENV NODE_OPTIONS=--max-old-space-size=4096
ENV VITE_WEB=true
RUN npm run build

# ============================================================
# Stage 2: Build Go backend (web mode)
# ============================================================
FROM golang:1.25-alpine AS backend-builder

WORKDIR /app
COPY go.mod go.sum ./
ENV GOPROXY=https://goproxy.cn,https://goproxy.io,direct
RUN GOFLAGS="-mod=mod" go mod download

COPY backend/ ./backend/
COPY main_web.go ./
# 版本号默认取自 VERSION 文件；发布时可用 APP_VERSION 覆盖
COPY version.go ./
COPY VERSION ./

ARG APP_VERSION=
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS="-mod=mod" go build -tags web -ldflags "-s -w ${APP_VERSION:+-X main.version=${APP_VERSION}}" -o /app/kvpilot-server .

# ============================================================
# Stage 3: Runtime (nginx + Go backend)
# ============================================================
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata nginx \
    && rm -rf /var/cache/apk/* /tmp/*

# Frontend static files
COPY --from=frontend-builder /app/frontend/dist /usr/share/nginx/html

# Nginx config
COPY docker/nginx.conf /etc/nginx/http.d/default.conf

# Go backend binary
WORKDIR /app
COPY --from=backend-builder /app/kvpilot-server .
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 8086

ENV PORT=8088
ENV GIN_MODE=release
ENV XDG_CONFIG_HOME=/app

ENTRYPOINT ["/entrypoint.sh"]
