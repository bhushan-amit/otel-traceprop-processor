# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A custom OpenTelemetry Collector distribution with a bespoke `tracepropagator` processor. The processor injects `TraceParentName` attributes into child spans by resolving parent span IDs to their span names — enabling downstream querying by root/parent operation name.

Deployed on AWS Elastic Beanstalk (`Otel-collector-env-1`), 4× c6g.2xlarge ARM64, behind an internal ALB.

---

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

---

## Architecture

### Two-module layout

The repo has two separate Go modules:

- `processor/tracepropagatorprocessor/` — the custom processor as a standalone module, developed and tested independently.
- `otelcol-dist/` — **generated** by `builder`; do not edit by hand. It wires all components (receivers, processors, exporters, connectors) into a runnable binary.

`otelcol-builder.yaml` is the source of truth for what goes into `otelcol-dist/`. Modify it to add/remove collector components, then re-run `builder` to regenerate.

### Collector pipeline (config.yaml) — current production state

```
Receivers              Processors                                    Exporters
─────────────────      ──────────────────────────────────────────    ──────────────────────────────
OTLP (4317/4318)  →   memory_limiter → batch → filter →         →   kafka/traces  (MSK otel-traces)
Host metrics           transform → tracepropagator                   kafka/logs    (MSK otel-logs)
Prometheus (8888)                                                     prometheusremotewrite (10.100.32.79:3100)
                       spanmetrics connector → prometheusremotewrite
                       servicegraph connector → prometheusremotewrite
```

**What is NOT in the pipeline (defined but unused):** `clickhouse`, `otlp/tempo`, `otlphttp/loki`, `otlphttp/grafana_cloud`, `otlp/hdx`. Do not add them back without confirming intent.

`config.yaml` is baked into the Docker image at `/etc/otel/config.yaml`. Editing the file on the instance has no effect — must rebuild image and redeploy.

### Custom processor: tracepropagator

`processor/tracepropagatorprocessor/processor.go` implements a two-pass algorithm inside `processTraces()`:

1. **First pass** — walk all spans and build a `spanID → spanName` map.
2. **Second pass** — for each span that has a `ParentSpanID`, look up the parent's name in the map and set `TraceParentName` as a span attribute.

Root spans and orphan spans (parent not found in the same batch) are left unmodified.

Factory registration follows the standard OTel processor pattern: `factory.go`'s `createTracesProcessor` calls `processorhelper.NewTraces` and passes `p.processTraces` as the consume callback.

> Note: the `tracePropagatorProcessor` struct also has `ConsumeTraces`, `Capabilities`, and `Shutdown` methods — these are vestigial and **not** on the live path. Edit `processTraces`, not `ConsumeTraces`.

---

## Infrastructure

### AWS resources

| Resource | Value |
|---|---|
| EB environment | `Otel-collector-env-1` |
| EB application | `otel-collector` (NOT `otel-collector-custom`) |
| ECR image | `869420678547.dkr.ecr.ap-south-1.amazonaws.com/otel-collector-custom:latest` |
| Region | `ap-south-1` |
| Instance type | `c6g.2xlarge` (ARM64, 8 vCPU, 16 GB RAM) |
| ASG name | `awseb-e-ddjypnzayt-stack-AWSEBAutoScalingGroup-J5RNVym4KNrC` |
| ALB (internal) | `internal-awseb--AWSEB-jd6qbz5rYzfm-767275064.ap-south-1.elb.amazonaws.com` |
| gRPC TG | `otel-grpc-4317-prod` — `arn:aws:elasticloadbalancing:ap-south-1:869420678547:targetgroup/otel-grpc-4317-prod/6da1acaa9a11fe40` |
| Health check TG | `awseb-healt-RAEEMLBXIIQB` (port 13133) |
| S3 deploy bucket | `elasticbeanstalk-ap-south-1-869420678547` |
| S3 deploy prefix | `otel-collector-custom/` |

### MSK Kafka

| Topic | Partitions | Consumer group (ClickStack) |
|---|---|---|
| `otel-traces` | 4 | `clickstack-otel-consumer` |
| `otel-logs` | 4 | `clickstack-otel-consumer` |

Brokers: `b-1.clickpostotelkafka.k5tbao.c2.kafka.ap-south-1.amazonaws.com:9092`, `b-2...`

Producer config (`config.yaml`): `partition_logs_by_resource_attributes: true` on `kafka/logs` — logs from the same service consistently hash to the same Kafka partition.

### gRPC Target Group attachment

`otel-grpc-4317-prod` is attached to the ASG — instances auto-register on launch and auto-deregister on termination. No manual TG registration needed. Health check path: `/grpc.health.v1.Health/Check` on port 4317.

---

## Memory management — critical

### The OOM problem (confirmed June 2026)

`otelcol-custom` on `c6g.2xlarge` (16 GB RAM) was OOM-killed at 14.7 GB RSS. The `memory_limiter` at 85% (`limit_percentage: 85`) only measures Go heap via `runtime.ReadMemStats`, not total RSS — so it fired too late.

### Current fix (in Dockerfile + config.yaml)

```
12 GB  — memory_limiter drops backpressure (limit_percentage: 75 × 16 GB)
13 GB  — GOMEMLIMIT=13000MiB: Go GC runs aggressively, prevents heap growth
16 GB  — OOM killer (should never reach this now)
```

- `Dockerfile`: `ENV GOMEMLIMIT=13000MiB`
- `config.yaml`: `limit_percentage: 75`, `spike_limit_percentage: 10`

If OOM recurs, check with: `aws ec2 get-console-output --instance-id <id> --region ap-south-1 --latest --query 'Output' --output text | grep -i oom`

### Zombie instance pattern

OOM kill → Docker restarts container (Restart: always) → 20-30s startup gap → gRPC health check times out → TG marks unhealthy → `OtelCollector-UnhealthyTargetsInGrpcTG` alarm fires.

If alarm fires: check TG health, identify the unhealthy instance, terminate it. ASG replaces it with a fresh instance that auto-registers into both TGs.

---

## Deployment procedure

Every config or code change requires a new Docker image. `Dockerrun.aws.json` changes (ports, restart policy) only need an EB bundle redeploy.

### Full deploy (config.yaml or Dockerfile changed)

```bash
# 1. Authenticate to ECR
aws ecr get-login-password --region ap-south-1 | \
  docker login --username AWS --password-stdin \
  869420678547.dkr.ecr.ap-south-1.amazonaws.com

# 2. Build ARM64 image and push
docker buildx build \
  --platform linux/arm64 \
  --tag 869420678547.dkr.ecr.ap-south-1.amazonaws.com/otel-collector-custom:latest \
  --push .

# 3. Bundle and deploy
cd eb-deploy
zip -j eb-bundle.zip Dockerrun.aws.json
LABEL="deploy-$(date +%Y%m%d_%H%M%S)"
aws s3 cp eb-bundle.zip \
  s3://elasticbeanstalk-ap-south-1-869420678547/otel-collector-custom/${LABEL}.zip \
  --region ap-south-1
aws elasticbeanstalk create-application-version \
  --region ap-south-1 \
  --application-name otel-collector \
  --version-label "${LABEL}" \
  --source-bundle S3Bucket=elasticbeanstalk-ap-south-1-869420678547,S3Key=otel-collector-custom/${LABEL}.zip
aws elasticbeanstalk update-environment \
  --region ap-south-1 \
  --environment-name Otel-collector-env-1 \
  --version-label "${LABEL}"
```

### Deployment gotchas

- **Application name is `otel-collector`**, not `otel-collector-custom`. Using the wrong name gives `InvalidParameterValue: No Application named 'otel-collector-custom' found`.
- **Terminate zombie instances before deploying.** EB picks instances in arbitrary order. If the first instance in a rolling batch is OOM-crashed (port 13133 not responding), EB times out and aborts the deploy, leaving instances in a mixed-version state. Always check `aws elbv2 describe-target-health` before deploying and terminate any `Target.Timeout` instances first.
- **After aborted deploy, redeploy to same version label** to get all instances consistent.
- **First build takes ~15 minutes** (Go compiles all modules). Subsequent builds use layer cache (~2 minutes).

### Verify deploy

```bash
# Environment health
aws elasticbeanstalk describe-environment-health \
  --region ap-south-1 --environment-name Otel-collector-env-1 \
  --attribute-names All \
  --query '{Health:HealthStatus,Status:Status,Ok:InstancesHealth.Ok}'

# Per-instance version
aws elasticbeanstalk describe-instances-health \
  --region ap-south-1 --environment-name Otel-collector-env-1 \
  --attribute-names Deployment \
  --query 'InstanceHealthList[*].{Id:InstanceId,Health:HealthStatus,Version:Deployment.VersionLabel}' \
  --output table

# gRPC TG health
aws elbv2 describe-target-health \
  --target-group-arn arn:aws:elasticloadbalancing:ap-south-1:869420678547:targetgroup/otel-grpc-4317-prod/6da1acaa9a11fe40 \
  --region ap-south-1 \
  --query 'TargetHealthDescriptions[*].{Id:Target.Id,State:TargetHealth.State,Reason:TargetHealth.Reason}' \
  --output table
```

### SSH / instance access

SSH via `eb ssh` requires the EB role to have `ec2:AuthorizeSecurityGroupIngress` — this is blocked. Use SSM instead:
```bash
aws ssm start-session --target <instance-id> --region ap-south-1
```

---

## CloudWatch alarms

| Alarm | Cause | Fix |
|---|---|---|
| `OtelCollector-UnhealthyTargetsInGrpcTG` | Instance OOM → container restart → port 4317 down during startup | Check TG health, terminate unhealthy instance |

---

## S3 log archival (otel_logs_to_s3.py)

Script on `/Users/amitbhushan/Desktop/otel_logs_to_s3.py`. Consumes `otel-logs` topic, writes gzip JSONL to `s3://clickpost-opentelemetry-mumbai/otel-logs/` partitioned by `{service}/dt={date}/hour={HH}/p{partition}-{MM}m{SS}s.jsonl.gz`.

- Consumer group: `otel-s3-archival-logs`
- `enable.auto.commit: False` — offset committed only after S3 upload succeeds
- Buffers per partition (not global) — safe to run multiple instances in the same consumer group
- Max parallelism = number of Kafka partitions (currently 4)
- Default flush: 3600s or 2M records, whichever comes first
- Test wrapper: `_run_60s.py` on Desktop (overrides `FLUSH_INTERVAL_SEC=55`)
- venv: `venv_otel_s3` on Desktop

At 3600s flush: ~1,500-1,700 S3 PUTs/hour across 4 partitions → ~$0.20/day.