FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/xueya .

FROM alpine:3.21

WORKDIR /app
RUN addgroup -S app && adduser -S app -G app && mkdir -p /app/data && chown -R app:app /app
COPY --from=builder /out/xueya /app/xueya

USER app
ENV ADDR=:6644
ENV DATA_PATH=/app/data/records.json
EXPOSE 6644
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:6644/api/records >/dev/null || exit 1

CMD ["/app/xueya"]
