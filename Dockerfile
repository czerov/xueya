FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY . .
ARG APP_VERSION=0.1.1
ARG GIT_COMMIT=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.appVersion=${APP_VERSION} -X main.gitCommit=${GIT_COMMIT}" -o /out/xueya .

FROM alpine:3.21

WORKDIR /app
RUN mkdir -p /app/data /app/config
COPY --from=builder /out/xueya /app/xueya

ENV ADDR=:6644
EXPOSE 6644
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:6644/api/health >/dev/null || exit 1

CMD ["/app/xueya"]
