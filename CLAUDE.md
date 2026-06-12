# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A custom OpenTelemetry Collector distribution with a bespoke `tracepropagator` processor. The processor injects `TraceParentName` attributes into child spans by resolving parent span IDs to their span names — enabling downstream querying by root/parent operation name.

## Commands

### Build

Install the builder tool (once):
```bash
go install go.opentelemetry.io/collector/cmd/builder@v0.123.0
```

> ⚠️ Version skew: `otelcol-builder.yaml` pins core/contrib components at a mix of
> `v0.123.0` and `v0.124.0`. If the build fails on incompatible module versions,
> align all entries to the same minor (and match the `builder` version to it)
> before re-running.

Build the collector binary from the manifest:
```bash
builder --config=otelcol-builder.yaml
# Output: ./otelcol-dist/otelcol-custom
```

Build the Docker image:
```bash
docker build -t otel-collector-custom .
```

### Run

Run locally:
```bash
./otelcol-dist/otelcol-custom --config=config.yaml
```

Generate test traces against a running collector:
```bash
telemetrygen traces --otlp-insecure --traces 1
```

### Test

Tests live in the processor module only:
```bash
cd processor/tracepropagatorprocessor
go test -v
```

Run a single test:
```bash
cd processor/tracepropagatorprocessor
go test -v -run TestProcessTraces_RootAndChildSpanPropagation
```
(Test funcs: `TestProcessTraces_RootAndChildSpanPropagation`, `TestProcessTraces_OnlyRootSpan`.)

## Architecture

### Two-module layout

The repo has two separate Go modules:

- `processor/tracepropagatorprocessor/` — the custom processor as a standalone module, developed and tested independently.
- `otelcol-dist/` — **generated** by `builder`; do not edit by hand. It wires all components (receivers, processors, exporters, connectors) into a runnable binary.

`otelcol-builder.yaml` is the source of truth for what goes into `otelcol-dist/`. Modify it to add/remove collector components, then re-run `builder` to regenerate.

### Collector pipeline (config.yaml)

```
Receivers          →  Processors                          →  Exporters
─────────────────     ────────────────────────────────────   ────────────────────────────
OTLP (4317/4318)      memory_limiter                         ClickHouse (traces, logs)
Host metrics          batch                                  Tempo via OTLP/gRPC
Prometheus (8888)     tracepropagator  ← custom              Prometheus Remote Write
                      resourcedetection                      Loki (logs)
                      transform / filter                     Grafana Cloud
                      groupbytrace / tailsampling            HDX
```

Connectors (`spanmetrics`, `servicegraph`, `grafanacloud`, `routing`) bridge between pipeline types.

### Custom processor: tracepropagator

`processor/tracepropagatorprocessor/processor.go` implements a two-pass algorithm inside `processTraces()`:

1. **First pass** — walk all spans and build a `spanID → spanName` map.
2. **Second pass** — for each span that has a `ParentSpanID`, look up the parent's name in the map and set `TraceParentName` as a span attribute.

Root spans and orphan spans (parent not found in the same batch) are left unmodified.

Factory registration follows the standard OTel processor pattern: `factory.go`'s `createTracesProcessor` calls `processorhelper.NewTraces` and passes `p.processTraces` as the consume callback. `config.go` defines an empty `Config` embedding `component.Config` (no tunable fields yet).

> Note: the `tracePropagatorProcessor` struct also has `ConsumeTraces`, `Capabilities`, and `Shutdown` methods — these are vestigial and **not** on the live path. The actual processing is `processTraces`, wired through `processorhelper.NewTraces`. Edit `processTraces`, not `ConsumeTraces`.

## Deployment

**Docker** — multi-stage build (`golang:1.24-bookworm` → `debian:bookworm-slim`). Runs as non-root `otel:otel`. Exposes 4317, 4318, 8888, 13133 (health).

**AWS Elastic Beanstalk** — see `eb-deploy/`. ECR image is `869420678547.dkr.ecr.ap-south-1.amazonaws.com/otel-collector-custom:1.0.x`. Bundle versions are tracked in `.elasticbeanstalk/`.

**systemd** — `otelcol-custom.service` runs the binary with 720% CPU quota and 90% memory limit, restarting on failure after 15 s.

Health check endpoint: `http://localhost:13133/`
