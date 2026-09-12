# 先构建 Nuxt 静态 SPA，再将其嵌入 Go 服务端二进制。
FROM node:24-bookworm-slim AS frontend-build

WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable \
  && pnpm install --frozen-lockfile --ignore-scripts \
  && pnpm rebuild esbuild msw vue-demi
COPY web ./
RUN pnpm generate

FROM golang:1.27-bookworm AS backend-build

WORKDIR /src
COPY go.mod go.sum ./
ENV GOPROXY=https://proxy.golang.org,direct
RUN go mod download
COPY . ./
COPY --from=frontend-build /src/web/.output/public ./web/.output/public
RUN CGO_ENABLED=0 go build -tags embed_web -trimpath -o /out/meridian ./cmd/meridian

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S meridian && adduser -S -G meridian meridian
COPY --from=backend-build /out/meridian /usr/local/bin/meridian

USER meridian
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/meridian"]
