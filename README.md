# otel-traceprop-processor v0.1.0

A custom OpenTelemetry Collector processor that injects additional metadata into trace spans as they flow through the pipeline.

This processor is built to demonstrate custom trace manipulation within the OpenTelemetry Collector framework.

---

## 🔧 On Machine Setup & Usage

### 1. Build the Custom Collector

Use the [OpenTelemetry Collector Builder](https://github.com/open-telemetry/opentelemetry-collector) to generate a custom binary with this processor included:

```bash
builder --config=otelcol-builder.yaml
```

This will generate the custom collector binary at: ``` ./otelcol-dist/otelcol-custom```

### 2. Run the Collector with Configuration

Start the collector using your built binary and a `config.yaml` that wires up the custom processor:

```bash
./otelcol-dist/otelcol-custom --config=config.yaml
```

### 3. Test with Generate Sample Traces

Send test traces to the collector using [`telemetrygen`](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/cmd/telemetrygen):

```bash
telemetrygen traces --otlp-insecure --traces 1
```

This will emit a single trace to the collector via OTLP gRPC on `localhost:4317`.

## ✅ Expected Behavior

If wired correctly, the custom processor will enrich each span with a static attribute (e.g., `hello=world`).  
You can verify this by observing logs in the console or from the configured exporter.


---
 
## 🐳 Docker Setup
 
### Build the Image
 
Builds for `linux/amd64` (AWS EB standard instances). Run from the repo root.
 
> On Apple Silicon (M1/M2/M3), Docker Buildx handles cross-compilation automatically.
 
```bash
docker buildx build \
  --platform linux/amd64 \
  --tag otel-collector-custom:1.0.0 \
  --load \
  .
```
 
> **First build takes ~15 minutes** — Go downloads all modules and compiles the binary.
> Subsequent builds are fast due to Docker layer caching.
 
### Run the Image Locally
 
```bash
docker run --rm \
  -p 4317:4317 \
  -p 4318:4318 \
  -p 13133:13133 \
  -p 8888:8888 \
  otel-collector-custom:1.0.0
```
 
Ports:
| Port  | Purpose                        |
|-------|--------------------------------|
| 4317  | OTLP gRPC receiver             |
| 4318  | OTLP HTTP receiver             |
| 8888  | Collector self-metrics         |
| 13133 | Health check                   |
 
### Verify the Collector is Healthy
 
```bash
curl http://localhost:13133/
```
 
Expected response:
```json
{"status":"Server available","upSince":"...","uptime":"..."}
```
 
### Send a Test Trace
 
```bash
curl -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{
    "resourceSpans": [{
      "resource": {
        "attributes": [{
          "key": "service.name",
          "value": {"stringValue": "test-service"}
        }]
      },
      "scopeSpans": [{
        "spans": [{
          "traceId": "5B8EFFF798038103D269B633813FC60C",
          "spanId": "EEE19B7EC3C1B174",
          "name": "test-span",
          "kind": 1,
          "startTimeUnixNano": "1640000000000000000",
          "endTimeUnixNano":   "1640000001000000000",
          "attributes": [{
            "key": "enterprise.id",
            "value": {"stringValue": "test-enterprise-123"}
          }]
        }]
      }]
    }]
  }'
```
 
Expected response: `{"partialSuccess":{}}` — empty partialSuccess means full success.
 
### Point a Local App at the Collector
 
From inside another Docker container on the same Mac:
 
```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://host.docker.internal:4317"
```
 
---
