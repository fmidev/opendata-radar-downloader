FROM golang:1.24-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o fmi-radar-downloader .

FROM alpine:3.21

RUN apk add --no-cache ca-certificates \
    && adduser -D -h /data appuser

COPY --from=builder /build/fmi-radar-downloader /usr/local/bin/

RUN mkdir -p /data && chown appuser:appuser /data
VOLUME /data

USER appuser
ENV OUTPUT_DIR=/data

HEALTHCHECK --interval=5m --timeout=5s --start-period=2m --retries=3 \
    CMD test -f /data/.last_successful_poll && \
        test "$(find /data/.last_successful_poll -mmin -10 2>/dev/null)" != ""

ENTRYPOINT ["fmi-radar-downloader"]
