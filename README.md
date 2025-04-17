# otel-traceprop-processor v0.1.0

A custom OpenTelemetry Collector processor that injects additional metadata into trace spans as they flow through the pipeline.

This processor is built to demonstrate custom trace manipulation within the OpenTelemetry Collector framework.

---

## 🔧 Setup & Usage

### 1. Build the Custom Collector

Use the [OpenTelemetry Collector Builder](https://github.com/open-telemetry/opentelemetry-collector) to generate a custom binary with this processor included:

```bash
builder --config=otelcol-builder.yaml --verbose
```

This will generate the custom collector binary at: ``` ./otelcol-dist/otelcol-custom```

### 2. Run the Collector with Configuration

Start the collector using your built binary and a `config.yaml` that wires up the custom processor:

```bash
./otelcol-dist/otelcol-custom --config=config.yaml --set=service.telemetry.logs.level=debug
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

