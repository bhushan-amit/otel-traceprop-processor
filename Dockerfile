# ──────────────────────────────────────────────────────────────
# Stage 1: Build
# ──────────────────────────────────────────────────────────────
FROM golang:1.24-bookworm AS builder

RUN go install go.opentelemetry.io/collector/cmd/builder@v0.123.0

WORKDIR /build

COPY otelcol-builder.yaml .
COPY processor/ ./processor/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    /go/bin/builder \
    --config=otelcol-builder.yaml \
    --output-path=/build/otelcol-dist

RUN ls -lh /build/otelcol-dist/otelcol-custom

# ──────────────────────────────────────────────────────────────
# Stage 2: Runtime
# ──────────────────────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends curl ca-certificates && \
    rm -rf /var/lib/apt/lists/*

RUN groupadd -r otel && useradd -r -g otel -s /bin/false otel

RUN mkdir -p /etc/otel /var/log/otel && \
    chown -R otel:otel /etc/otel /var/log/otel

COPY --from=builder /build/otelcol-dist/otelcol-custom /usr/local/bin/otelcol-custom
RUN chmod +x /usr/local/bin/otelcol-custom

COPY config.yaml /etc/otel/config.yaml

USER otel

EXPOSE 4317
EXPOSE 4318
EXPOSE 8888
EXPOSE 13133

HEALTHCHECK --interval=15s --timeout=5s --start-period=30s --retries=3 \
    CMD curl -sf http://localhost:13133/ || exit 1

ENTRYPOINT ["/usr/local/bin/otelcol-custom"]
CMD ["--config=/etc/otel/config.yaml", "--set=service.telemetry.logs.level=warn"]