# LOCAL.md — Otel Collector: Everything We Know

> Not committed to git. This is a running dump of all learnings, decisions, reference values,
> commands, gotchas, and operational knowledge accumulated over time.

---

## Table of Contents

1. [AWS Infrastructure Map](#1-aws-infrastructure-map)
2. [Instance Sizing & Cost Analysis](#2-instance-sizing--cost-analysis)
3. [Billing Reference — Jul 7 2026 Actuals](#3-billing-reference--jul-7-2026-actuals)
4. [CloudWatch Dashboard](#4-cloudwatch-dashboard)
5. [CWAgent — Memory Metrics](#5-cwagent--memory-metrics)
6. [Autoscaling Analysis](#6-autoscaling-analysis)
7. [ClickHouse — Disk & Operations](#7-clickhouse--disk--operations)
8. [Public ALB for Frontend (OTLP/HTTP 4318)](#8-public-alb-for-frontend-otlphttp-4318)
9. [Session Replay (rrweb) Pipeline](#9-session-replay-rrweb-pipeline)
10. [transfer_log_to_s3 — Log Archival Cron](#10-transfer_log_to_s3--log-archival-cron)
11. [Config Changes Made](#11-config-changes-made)
12. [Makefile — Build & Deploy](#12-makefile--build--deploy)
13. [Operational Commands Reference](#13-operational-commands-reference)
14. [Gotchas & Learnings](#14-gotchas--learnings)

---

## 1. AWS Infrastructure Map

### Elastic Beanstalk

| Field | Value |
|---|---|
| EB Application | `otel-collector` (NOT `otel-collector-custom` — this trips you up) |
| EB Environment | `Otel-collector-env-1` |
| Region | `ap-south-1` |
| ECR Image | `869420678547.dkr.ecr.ap-south-1.amazonaws.com/otel-collector-custom:latest` |
| S3 Deploy Bucket | `elasticbeanstalk-ap-south-1-869420678547` |
| S3 Deploy Prefix | `otel-collector-custom/` |
| ASG | `awseb-e-ddjypnzayt-stack-AWSEBAutoScalingGroup-J5RNVym4KNrC` |

### ALBs

| ALB | DNS | Purpose |
|---|---|---|
| Internal (gRPC) | `internal-awseb--AWSEB-jd6qbz5rYzfm-767275064.ap-south-1.elb.amazonaws.com` | Trace ingestion from backend services |
| Public (HTTP) | `otel-http.clickpost.in` | Browser/frontend OTLP/HTTP 4318 |

ALB ARN (internal): `app/awseb--AWSEB-jd6qbz5rYzfm/79b525ff4c677230`

### Target Groups

| TG Name | ARN | Port | Protocol |
|---|---|---|---|
| `otel-grpc-4317-prod` | `arn:aws:elasticloadbalancing:ap-south-1:869420678547:targetgroup/otel-grpc-4317-prod/6da1acaa9a11fe40` | 4317 | gRPC |
| `awseb-otlph-TV2NIWXR0T7Z` | `arn:aws:elasticloadbalancing:ap-south-1:869420678547:targetgroup/awseb-otlph-TV2NIWXR0T7Z/d6a2cfaed618374c` | 4318 | HTTP |
| `awseb-healt-RAEEMLBXIIQB` | (health check) | 13133 | HTTP |

### EC2 Instances (as of Oct 2026 — 11 × c6g.2xlarge)

| Instance ID | Notes |
|---|---|
| i-0a11eb647137895c0 | ASG member |
| i-0cea4855d2d9d76db | ASG member |
| i-067f9c77ce30ff77e | ASG member |
| i-04b127fa2b5812371 | ASG member |
| i-0b346e7c30d82c1e1 | ASG member |
| i-0bd0230603551cd88 | ASG member |
| i-0a1c214fb0754a1d1 | ASG member |
| i-0f09ae0cb63314eab | ASG member |
| i-0cf033f8746e76a56 | ASG member |
| i-0dd5c7d1b3e449a9e | ASG member |
| i-065a6c0124e679747 | ASG member |

Instance IDs change on ASG replacement — always re-check with `aws elasticbeanstalk describe-instances-health` before SSM. CWAgent installed via SSM Run Command (Jul 2026); use `AWS-ConfigureAWSPackage` then `AmazonCloudWatch-ManageAgent`.

### MSK Kafka

Brokers:
- `b-1.clickpostotelkafka.k5tbao.c2.kafka.ap-south-1.amazonaws.com:9092`
- `b-2.clickpostotelkafka.k5tbao.c2.kafka.ap-south-1.amazonaws.com:9092`

Topics:
| Topic | Partitions | Use |
|---|---|---|
| `otel-traces` | **16** | All traces → ClickStack consumer. Expanded from 4 → 16 in Oct 2026. |
| `otel-logs` | 24 | Application logs → ClickStack consumer |
| `otel-rrweb` | — | Session replay events → (consumer TBD) |

**Kafka is NOT used for metrics.** Metrics go direct to prometheusremotewrite (`10.100.40.110:8080/api/v1/write`).

**otel-traces partitioner:** `record_partitioner: least_backup` (v0.162.0 franz-go). Distributes evenly across all 16 partitions. Previously: Sarama nil-key hash → all traffic concentrated on one partition (P11 after 4→16 expansion). Do NOT revert to default Sarama behaviour.

### ClickHouse

Internal endpoint: `http://10.100.46.16:8123`  
HyperDX (HDX) endpoint: `10.100.46.16:4317`  
Database: `default`  
Tables: `otel_traces`, `otel_logs`, and various scylla/system metric tables.

Tiered storage: hot EBS → cold S3 via TTL (`TimestampTime + 24h TO VOLUME 'cold'`). Hot disk triggers move at 80% full (`move_factor=0.2`).

---

## 2. Instance Sizing & Cost Analysis

### Current: 7 × c6g.2xlarge

| Spec | Value |
|---|---|
| vCPU | 8 |
| RAM | 16 GB |
| Network | Up to 10 Gbps |
| Storage | EBS only |
| On-demand (ap-south-1) | **$0.136/hr** |

**Observed utilization (Jun–Jul 2026):**
- Memory: 81–84% → 13–13.4 GB used out of 16 GB — **almost no headroom**
- CPU: 50–65%
- Memory is the bottleneck, not CPU.

### Why 7 machines?

Deepanshu raised this — 7 × c6g.2xlarge feels like overkill. Reality: OTel is I/O heavy. Memory fills up from the in-flight queue and buffer management in the collector process. At ~136 GB/hr ALB ingestion, each instance processes ~19 GB/hr.

### Recommended upgrade path

Switch to **m6g.2xlarge** (same 8 vCPU, 32 GB RAM, $0.154/hr):
- Memory drops from 81–84% → ~40–42%
- Can scale down from 7 → 4–5 instances and still have headroom
- Net cost: 5 × $0.154 × 730 = $562/mo vs current 7 × $0.136 × 730 = $695/mo → **saves ~$133/mo while halving memory pressure**

Comparison table:
| Instance | vCPU | RAM | $/hr (ap-south-1) |
|---|---|---|---|
| c6g.2xlarge (current) | 8 | 16 GB | $0.136 |
| m6g.2xlarge (recommended) | 8 | 32 GB | $0.154 |
| r6g.xlarge | 4 | 32 GB | $0.126 |
| r6g.2xlarge | 8 | 64 GB | $0.253 |

To change instance type in EB: create a new launch template with the target instance type, update the EB environment to use it, then trigger a rolling deploy.

---

## 3. Billing Reference — Jul 7 2026 Actuals

### ALB (ProcessedBytes dominant dimension)

| Metric | Value |
|---|---|
| Average GB/hr | **135.83 GB/hr** |
| Peak GB/hr | **194.79 GB/hr** |
| Total for day | 3,259 GB |
| ALB daily cost | **$26.08/day** (`total_GB × $0.008`) |
| ALB monthly projected | **~$794/mo** (`avg_GB/hr × $0.008 × 730`) |

**Key formula correction:** ALB billing is per GB/hr (1 LCU = 1 GB processed per hour). Daily cost = total_GB_that_day × $0.008. Do NOT multiply by 24 again.

Pre-gzip (Jun 7): ~1,049 GB/hr → ~$6.15/hr → ~$4,490/mo  
Post-gzip (Jun 10): ~99 GB/hr (initial measurement)  
Current (Jul 7): ~136 GB/hr (stable steady-state)

### EC2

| Metric | Value |
|---|---|
| Instance type | c6g.2xlarge |
| On-demand rate | $0.136/hr |
| Count | 7 |
| Monthly (7 inst) | **$695/mo** |
| Monthly (10 inst max) | ~$997/mo |

### EBS

~$5/mo total (7 × 8 GB GP3 at $0.08/GB-mo). Negligible.

### Total monthly

| Component | Cost |
|---|---|
| ALB | ~$794 |
| EC2 (7 inst) | ~$695 |
| EBS | ~$5 |
| **Total** | **~$1,494/mo** |

Historical comparison: before gzip compression the ALB alone was ~$4,490/mo.

---

## 4. CloudWatch Dashboard

Dashboard name: **`Otel-collector-env-1`**  
Last updated: Jul 8 2026

### Panels in order (top → bottom)

**Row: Billing Signals**
- ConsumedLCUs (ALB billing units live)
- ProcessedBytes GB/hr — annotation at Jul 7 avg 136 GB/hr
- ALB Cost $/hr and $/mo — annotation at ~$1.09/hr
- EC2 Cost — `HealthyHostCount × $0.136` — annotation at 7 inst ref

**Row: Compression Impact (gzip rollout Jun 9)**
- ProcessedBytes 7-day trend — vertical annotation at gzip start, horizontal at 1,049 (pre) and 136 (post/current)
- NewConnectionCount (2nd billing dimension)

**Row: Request Throughput**
- gRPC RequestCount (4317)
- OTLP/HTTP RequestCount (4318)
- ActiveConnectionCount (3rd billing dimension)

**Row: EC2 Fleet Health**
- CPU Utilization (ASG avg)
- **Memory Used % — All 7 Instances (CWAgent)** ← new Jul 8
- NetworkIn/NetworkOut GB/hr per instance — annotation at 0.49 GB/hr peak
- HealthyHostCount vs UnhealthyHostCount (both TGs)

**Row: Errors, Latency & Availability**
- 5XX Errors
- 4XX Errors
- Target Response Time p50/p99
- TLS Errors + EC2 Status Check Failures

**Row: Billing Formula Reference**
- ALB pricing table + Jul 7 actuals
- EC2 pricing table + total monthly estimate + m6g.2xlarge upgrade note

### To update dashboard

```bash
aws cloudwatch put-dashboard \
  --region ap-south-1 \
  --dashboard-name "Otel-collector-env-1" \
  --dashboard-body file:///path/to/dashboard.json
```

---

## 5. CWAgent — Memory Metrics

### Why needed

`otelcol_process_memory_rss` only shows Go heap (~330–540 MB). The collector process RSS is actually 13–14 GB (Go runtime + OS buffers + Kafka/gRPC internal queues). CWAgent reads `/proc/meminfo` to get real system memory.

### CWAgent installation (Jul 2026)

Installed via SSM on all 7 instances. Config pushed:

```json
{
  "metrics": {
    "metrics_collected": {
      "mem": {
        "measurement": ["mem_used_percent"],
        "metrics_collection_interval": 60
      }
    },
    "append_dimensions": {
      "AutoScalingGroupName": "${aws:AutoScalingGroupName}"
    }
  }
}
```

### CWAgent metric query (critical: requires BOTH dimensions)

```
Namespace: CWAgent
Metric: mem_used_percent
Dimensions:
  InstanceId: <id>
  AutoScalingGroupName: awseb-e-ddjypnzayt-stack-AWSEBAutoScalingGroup-J5RNVym4KNrC
```

**Querying with only InstanceId returns no data.** Both dimensions are required because `append_dimensions` adds AutoScalingGroupName to the metric key.

### Baseline (Jul 8 2026, morning)

All instances: **81–83% memory used** out of 16 GB (i.e., 13–13.3 GB).

### To check memory across all instances

```bash
for iid in i-08092e802a306c3ce i-02ef4fc7aede3421b i-066137a07ad35335d \
           i-09cfeb4e0aa59e920 i-078d5778656cd2232 i-0a8d447e97e76cd83 i-00e4558b4a0ef8731; do
  val=$(aws cloudwatch get-metric-statistics \
    --region ap-south-1 \
    --namespace CWAgent \
    --metric-name mem_used_percent \
    --dimensions Name=InstanceId,Value=$iid \
      Name=AutoScalingGroupName,Value=awseb-e-ddjypnzayt-stack-AWSEBAutoScalingGroup-J5RNVym4KNrC \
    --start-time $(date -u -v-1H +%Y-%m-%dT%H:%M:%SZ) \
    --end-time $(date -u +%Y-%m-%dT%H:%M:%SZ) \
    --period 3600 --statistics Average \
    --query 'Datapoints[0].Average' --output text 2>/dev/null)
  echo "$iid: ${val}%"
done
```

---

## 6. Autoscaling Analysis

### Current scaling trigger

EB default: **CPU-based**. At 50–65% CPU it never scales. Memory (the actual bottleneck) is not a native EC2 CloudWatch metric.

### NetworkIn as scaling signal

Deepanshu pointed out OTel is I/O-based — `NetworkIn` reflects incoming load (data arriving). It's proactive (reflects cause), whereas memory is reactive (reflects consequence after buffering).

### Jul 7 NetworkIn baseline (per-instance average over ASG)

| Time (IST) | Avg MB/hr | Avg MB/min | Max MB/hr |
|---|---|---|---|
| 11:20 (peak) | 468 | 7.8 | 574 |
| 12:20 (peak) | 470 | 7.8 | 537 |
| 09:20 (peak) | 441 | 7.4 | 523 |
| 22:20 (night) | 272 | 4.5 | 300 |
| 01:20 (night) | 232 | 3.9 | 274 |
| 03:20 (night) | 215 | 3.6 | 241 |

**Pattern:** Business hours 440–500 MB/hr, night 210–250 MB/hr. Clear diurnal cycle.

### Recommended autoscaling thresholds (not yet implemented)

| Action | Trigger | Rationale |
|---|---|---|
| Scale UP | NetworkIn > 550 MB/hr (avg, 5 min) | Approaching peak load |
| Scale DOWN | NetworkIn < 220 MB/hr (avg, 10 min) | Sustained night-level traffic |
| Scale DOWN cooldown | 10 min | Avoid thrashing at end of business day |

Memory as secondary safety net (via CWAgent custom metric alarm):
- Alert at 85%, scale up at 88% (can't directly trigger autoscaling with CWAgent metrics without custom metric alarm + step scaling policy)

---

## 7. ClickHouse — Disk & Operations

### Hot disk crisis (Jun–Jul 2026)

Hot disk hit 79.4% → triggered at `move_factor=0.2` (80% threshold). Root cause: system log tables (`query_log`, `trace_log`, etc.) had no TTL and were growing unbounded.

### System table TTLs — fix applied

```sql
ALTER TABLE system.query_log MODIFY TTL event_date + INTERVAL 3 DAY;
ALTER TABLE system.text_log MODIFY TTL event_date + INTERVAL 3 DAY;
ALTER TABLE system.trace_log MODIFY TTL event_date + INTERVAL 3 DAY;
-- (applied to ~10 system tables)
```

After applying: disk dropped from 79.4% → 75.5% → stabilized around 76%.

### MATERIALIZE TTL — use with caution

```sql
ALTER TABLE otel_traces MATERIALIZE TTL;
```

This forces immediate TTL application on all existing parts. **It is heavy** — flooded the background merge pool on our cluster (41 tasks vs max 28). Had to kill all 4 mutations:

```sql
-- Find running mutations
SELECT database, table, mutation_id, command, create_time
FROM system.mutations
WHERE is_done = 0;

-- Kill them
KILL MUTATION WHERE database='default' AND table='otel_traces' AND mutation_id='0000000001';
-- repeat for each table
```

After killing: let background TTL run naturally. Disk stabilized without forced merge.

### Background merge pool check

```sql
SELECT * FROM system.metrics WHERE metric LIKE '%BackgroundMerge%';
-- A=28 is max capacity. If A > 28, pool is saturated.
```

### Useful ClickHouse disk queries

```sql
-- Hot disk usage by table
SELECT
    table,
    formatReadableSize(sum(bytes_on_disk)) AS disk_size,
    formatReadableSize(sum(data_uncompressed_bytes)) AS uncompressed
FROM system.parts
WHERE active = 1 AND disk_name = 'default'
GROUP BY table
ORDER BY sum(bytes_on_disk) DESC
LIMIT 20;

-- System log table sizes
SELECT table, formatReadableSize(total_bytes) AS size
FROM system.tables
WHERE database = 'system'
ORDER BY total_bytes DESC;

-- Parts pending TTL move (hot → cold)
SELECT table, count() AS parts, formatReadableSize(sum(bytes_on_disk)) AS size
FROM system.parts
WHERE active = 1 AND move_ttl_info.expression != ''
GROUP BY table;

-- Check running mutations
SELECT database, table, mutation_id, command, parts_to_do, is_done
FROM system.mutations
WHERE is_done = 0;
```

### Scylla metrics in ClickHouse

```sql
-- Find scylla metric names
SELECT DISTINCT MetricName
FROM default.otel_metrics_gauge
WHERE MetricName LIKE '%scylla%'
LIMIT 50;

-- Latest scylla metric values
SELECT Timestamp, MetricName, Value, Attributes
FROM default.otel_metrics_gauge
WHERE MetricName LIKE '%scylla%'
ORDER BY Timestamp DESC
LIMIT 100;
```

---

## 8. Public ALB for Frontend (OTLP/HTTP 4318)

### What was built

A public-facing HTTPS endpoint for browser-side OTel SDK to send telemetry (traces, session replay logs) without going through the internal ALB.

| Resource | Value |
|---|---|
| DNS | `otel-http.clickpost.in` |
| Protocol | HTTPS/443 → HTTP/4318 (target) |
| ACM Cert | `*.clickpost.in` (existing, reused) |
| Target Group | `awseb-otlph-TV2NIWXR0T7Z` (port 4318) |
| ALB type | Internet-facing, public |

### DNS gotcha — Cloudflare, not Route53

`clickpost.in` nameservers are Cloudflare. Route53 records for `clickpost.in` are **ignored publicly** — they only matter for private hosted zones. Any DNS changes for `otel-http.clickpost.in` must be made in **Cloudflare**.

ACM certificate validation for `clickpost.in` also requires the CNAME validation record to be added in Cloudflare (not Route53).

### ACM cert lessons

- Created cert for `otel-http.internalcp.in` first — wrong. `internalcp.in` is a private Route53 zone, ACM can't validate it publicly.
- Deleted and reused existing `*.clickpost.in` cert instead.
- Validation CNAME must go into Cloudflare DNS, not Route53.

### CORS config (config.yaml)

```yaml
http:
  endpoint: 0.0.0.0:4318
  cors:
    allowed_origins:
      - "*"
    allowed_headers:
      - "*"
  include_metadata: true
```

---

## 9. Session Replay (rrweb) Pipeline

### Architecture

Browser records DOM events via `rrweb`, wraps each event as an OTel log with `rr-web.event` attribute set. Shipped to `otel-http.clickpost.in:443`.

In the collector:

```
logs/in → routing/logs connector
  ├─ if has rr-web.event attribute → logs/out-rrweb → kafka/rrweb → otel-rrweb topic
  └─ else → logs/out-default → kafka/logs → otel-logs topic
```

`otel-rrweb` is a **dedicated Kafka topic** — keeps bursty session replay events from creating consumer lag on `otel-logs`.

### Routing config (config.yaml)

```yaml
connectors:
  routing/logs:
    default_pipelines: [logs/out-default]
    error_mode: ignore
    table:
      - context: log
        statement: route() where IsMatch(attributes["rr-web.event"], ".*")
        pipelines: [logs/out-rrweb]
```

### What's pending

- Frontend SDK integration (rrweb + OTel browser SDK) not yet implemented in frontend repo
- ClickStack consumer for `otel-rrweb` topic not yet configured (no `otelcol-consumer` reading `otel-rrweb`)

---

## 10. transfer_log_to_s3 — Log Archival Cron

### What the cron does

Runs every 5 minutes on all prod machines. Scans `/var/log` for rotated log files (matching timestamp regex patterns), uploads to S3, then deletes the local file. It acts as **both log shipper AND log rotator** — the deletion is the rotation.

### Where the code lives

`/Users/amitbhushan/Repos/clickpost/utility/transfer_log_file_to_s3_utils.py`  
Cron runner: `transfer_log_to_s3_cron.py`

S3 bucket: `clickpost-logs-bucket-mumbai`  
S3 path pattern: `logs/{script_name}/{date}/{filename}`

### Change made: disable upload, keep deletion

Goal: stop pushing logs to S3 (cost + unused) but retain the local rotation (deletion) so logs don't accumulate on disk.

Control env var: `UPLOAD_LOG_FILES_S3=False` (default). Set to `"true"` or `"1"` to re-enable upload in emergency.

**`get_files()` change:** when upload disabled, skip S3 path construction entirely — still returns files for deletion but with `name=None`.

**`upload_s3files()` change:** when upload disabled, skip the S3 upload call but still call `os.remove()`. When upload enabled, upload then delete (original behavior).

The `ignore_script_names` list skips system logs: `ubuntu-advantage`, `alternatives`, `unattended-upgrades`, `kern`, `dpkg`, etc.

### PR not yet merged

Code changed locally in clickpost repo. PR description: "Disable S3 log upload while preserving rotated log deletion to prevent disk accumulation. Upload can be re-enabled via UPLOAD_LOG_FILES_S3 env var."

---

## 11. Config Changes Made

### v0.162.0 upgrade (Oct 2026)

Collector upgraded from v0.124.0 → v0.162.0. Kafka exporter switched from Sarama → franz-go.

Key breaking changes fixed:
- `kafka/traces`: `topic`/`encoding` moved into `traces:` sub-config; added `record_partitioner: least_backup: {}`
- `kafka/logs`, `kafka/rrweb`: `topic`/`encoding` moved into `logs:` sub-config
- `prometheusremotewrite/prod`: `endpoint`/`headers` moved into `http:` block; `resource_to_telemetry_conversion: enabled: true` → `resource_constant_labels: included: ["*"]`
- `tracepropagator: {}` removed from processors and traces pipeline
- Dockerfile: `golang:1.24-bookworm` → `golang:1.26-bookworm`; builder `@v0.123.0` → `@v0.162.0`
- OTTL paths auto-corrected at runtime (non-fatal warnings): `attributes[x]` → `span.attributes[x]` / `resource.attributes[x]` / `log.attributes[x]`

Ubuntu machine (`ip-10-100-33-39`) also runs the collector as a service (`otelcol-custom.service`) and was upgraded first; binary rebuilt with `go install go.opentelemetry.io/collector/cmd/builder@v0.162.0`.

### batch/clickhouse_metrics (added)

```yaml
batch/clickhouse_metrics:
  send_batch_size: 20000
  send_batch_max_size: 50000
  timeout: 15s
```

### metrics/clickhouse pipeline — filter BEFORE batch

```yaml
metrics/clickhouse:
  receivers: [otlp, spanmetrics]
  processors: [memory_limiter, filter/clickhouse_metrics, batch/clickhouse_metrics]
  exporters: [clickhouse]
```

**Why filter before batch:** `filter/clickhouse_metrics` drops ~70% of metrics (keeps only `^scylla.*`, `^system\..*`, `^process\..*`, `^cpython\..*`, `^traces.*`). Filtering first means the batch buffer only holds the 30% that will be exported — saves memory.

### filter/clickhouse_metrics — includes

```yaml
filter/clickhouse_metrics:
  error_mode: ignore
  metrics:
    include:
      match_type: regexp
      metric_names:
        - ^scylla.*
        - ^system\..*
        - ^process\..*
        - ^cpython\..*
        - ^traces.*
```

The `^traces.*` pattern was added by the user to capture spanmetrics-generated trace metrics.

### Deployment status

Changes in `config.yaml` require `make all` to rebuild Docker image and redeploy to EB.

---

## 12. Makefile — Build & Deploy

Location: `otel-traceprop-processor/Makefile`

```makefile
ECR_REPO   := 869420678547.dkr.ecr.ap-south-1.amazonaws.com/otel-collector-custom
EB_APP     := otel-collector
EB_ENV     := Otel-collector-env-1
S3_BUCKET  := elasticbeanstalk-ap-south-1-869420678547
S3_PREFIX  := otel-collector-custom
REGION     := ap-south-1
```

### Targets

| Target | What it does |
|---|---|
| `make build` | `docker buildx build --platform linux/arm64 --tag ECR_REPO:latest --push` |
| `make bundle` | Zips `.platform/` hooks + `eb-deploy/Dockerrun.aws.json` into `eb-bundle.zip` |
| `make deploy` | Runs `bundle`, uploads zip to S3, creates EB app version, triggers rolling deploy |
| `make ecr-login` | `aws ecr get-login-password \| docker login` |
| `make all` | `ecr-login → build → deploy` (full pipeline) |
| `make status` | Shows EB environment health + gRPC TG instance states |

**Important (Oct 2026):** `deploy` now depends on `bundle` which includes the `.platform/` directory (Wazuh SIEM agent + nginx realip hooks). These hooks deploy onto each EB instance and run as pre/post-deploy scripts. Do not strip `.platform/` from the bundle.

### AWS CLI pager gotcha

AWS CLI opens a pager for JSON output and blocks the deploy. Either:
- Press `q` to exit the pager and the deploy continues
- Or set `export AWS_PAGER=""` in your shell to disable globally

---

## 13. Operational Commands Reference

### EB status

```bash
# Quick health
aws elasticbeanstalk describe-environment-health \
  --region ap-south-1 --environment-name Otel-collector-env-1 \
  --attribute-names All \
  --query '{Health:HealthStatus,Status:Status,Ok:InstancesHealth.Ok,Degraded:InstancesHealth.Degraded}'

# Per-instance version check
aws elasticbeanstalk describe-instances-health \
  --region ap-south-1 --environment-name Otel-collector-env-1 \
  --attribute-names Deployment \
  --query 'InstanceHealthList[*].{Id:InstanceId,Health:HealthStatus,Version:Deployment.VersionLabel}' \
  --output table
```

### Target group health

```bash
# gRPC TG
aws elbv2 describe-target-health \
  --target-group-arn arn:aws:elasticloadbalancing:ap-south-1:869420678547:targetgroup/otel-grpc-4317-prod/6da1acaa9a11fe40 \
  --region ap-south-1 \
  --query 'TargetHealthDescriptions[*].{Id:Target.Id,State:TargetHealth.State,Reason:TargetHealth.Reason}' \
  --output table

# HTTP TG
aws elbv2 describe-target-health \
  --target-group-arn arn:aws:elasticloadbalancing:ap-south-1:869420678547:targetgroup/awseb-otlph-TV2NIWXR0T7Z/d6a2cfaed618374c \
  --region ap-south-1 \
  --query 'TargetHealthDescriptions[*].{Id:Target.Id,State:TargetHealth.State}' \
  --output table
```

### SSM into an instance

```bash
aws ssm start-session --target i-08092e802a306c3ce --region ap-south-1
# Then inside: sudo su, docker ps, docker logs <container_id>
```

Note: `eb ssh` is blocked (requires security group ingress permission denied to EB role). Always use SSM.

### ClickHouse — connect

```bash
# From within the VPC (e.g. via SSM on a collector instance)
curl "http://10.100.46.16:8123/?query=SELECT+1"
# Or with clickhouse-client
clickhouse-client --host 10.100.46.16 --port 9000
```

### Check ALB ProcessedBytes (today)

```bash
aws cloudwatch get-metric-statistics \
  --region ap-south-1 \
  --namespace AWS/ApplicationELB \
  --metric-name ProcessedBytes \
  --dimensions Name=LoadBalancer,Value=app/awseb--AWSEB-jd6qbz5rYzfm/79b525ff4c677230 \
  --start-time $(date -u -v-24H +%Y-%m-%dT%H:%M:%SZ) \
  --end-time $(date -u +%Y-%m-%dT%H:%M:%SZ) \
  --period 3600 --statistics Sum \
  --query 'sort_by(Datapoints, &Timestamp)[*].{t:Timestamp,GB:Sum}' \
  --output json | python3 -c "
import json,sys
data=json.load(sys.stdin)
total=sum(d['GB'] for d in data)
avg=total/len(data)/1e9 if data else 0
print(f'Avg: {avg:.2f} GB/hr')
print(f'Daily cost: \${total/1e9*0.008:.2f}')
print(f'Monthly proj: \${avg*0.008*730:.2f}')
"
```

### NetworkIn baseline (last 24h)

```bash
aws cloudwatch get-metric-statistics \
  --region ap-south-1 \
  --namespace AWS/EC2 \
  --metric-name NetworkIn \
  --dimensions Name=AutoScalingGroupName,Value=awseb-e-ddjypnzayt-stack-AWSEBAutoScalingGroup-J5RNVym4KNrC \
  --start-time $(date -u -v-24H +%Y-%m-%dT%H:%M:%SZ) \
  --end-time $(date -u +%Y-%m-%dT%H:%M:%SZ) \
  --period 3600 --statistics Average,Maximum \
  --output json | python3 -c "
import json,sys
data=sorted(json.load(sys.stdin)['Datapoints'], key=lambda x:x['Timestamp'])
for d in data:
    print(f\"{d['Timestamp']}: avg {d['Average']/1e6:.0f} MB, max {d['Maximum']/1e6:.0f} MB\")
"
```

---

## 14. Gotchas & Learnings

### EB / Deploy

- **EB application name is `otel-collector`**, not `otel-collector-custom`. The ECR repo is `otel-collector-custom` but the EB app is `otel-collector`. Every time you forget this, the CLI gives `No Application named 'otel-collector-custom' found`.
- **Terminate zombie instances before deploying.** EB rolling deploy picks a batch. If the first batch instance is OOM-crashed, EB waits for health check, times out, aborts deploy. Leaves mixed versions. Always check TG health first.
- **`make all` opens AWS CLI pager** — just press `q`. Add `export AWS_PAGER=""` to `~/.zshrc` to stop it permanently.
- **c8g.large vs c6g.2xlarge confusion**: old environment used `c8g.large` (Graviton4). New environment (`Otel-collector-env-1`) uses `c6g.2xlarge` (Graviton3). Price is very different: $0.05398 vs $0.136.

### Memory / GOMEMLIMIT

- `otelcol_process_memory_rss` only shows Go heap — misleading. Actual RSS is 13–14 GB.
- `GOMEMLIMIT=13000MiB` in Dockerfile sets Go GC ceiling. Without it, Go defers GC and RSS balloons until OOM kill.
- `memory_limiter` at `limit_percentage: 75` triggers at 12 GB (75% of 16 GB) and drops data. This is correct — prefer dropping to OOM-kill.

### ClickHouse

- **MATERIALIZE TTL on large tables saturates background merge pool.** Max pool = 28 tasks. Our 4-table MATERIALIZE TTL pushed it to 41. Kill the mutations, let background TTL run on its own schedule.
- **System tables have no TTL by default.** `query_log`, `text_log`, `trace_log` etc. grow forever unless you explicitly set TTL.
- **Hot disk move_factor=0.2** means ClickHouse starts moving parts to S3 cold when hot disk is 80% full. Alert at 79% = expected. The disk typically self-heals over hours as TTL moves data to cold.

### Cloudflare / DNS

- `clickpost.in` nameservers are Cloudflare. Route53 records for this domain are **completely ignored** by the public internet. ACM validation CNAMEs and any public DNS records must go into Cloudflare, not Route53.
- Route53 records for `clickpost.in` only matter for resources inside the VPC resolving via the private hosted zone.

### CWAgent / Memory Metrics

- Querying `CWAgent / mem_used_percent` with only `InstanceId` dimension returns **no data**. Must include both `InstanceId` AND `AutoScalingGroupName`.
- CWAgent was installed Jul 8 2026. No historical memory data before that date.

### ALB Billing Formula

- **1 LCU = 1 GB processed per hour** (not per total).
- Daily cost = `total_GB_for_day × $0.008` (simple).
- Monthly projection = `avg_GB_per_hr × $0.008 × 730`.
- Do NOT do `total_GB × $0.008 × 24` — that multiplies by 24 twice.
- ProcessedBytes is almost always the dominant LCU dimension for gRPC trace ingestion. NewConnections and ActiveConnections are far below threshold.

### SSM

- `eb ssh` is blocked — EB role lacks `ec2:AuthorizeSecurityGroupIngress`.
- SSM does work: `aws ssm start-session --target <instance-id> --region ap-south-1`.
- Install CWAgent via SSM: use `AWS-ConfigureAWSPackage` document first, then `AmazonCloudWatch-ManageAgent` to push config and start.

### transfer_log_to_s3

- The cron runs on all prod machines from an AMI-baked script. It's NOT OTel-collector specific.
- The `UPLOAD_LOG_FILES_S3=False` env var existed but was checked in the wrong scope — the Python class didn't read it. Fixed in our PR.
- The cron is the only thing rotating logs on those machines — disabling deletion would fill disk.

### Kafka partitioning

- **Never use `partition_traces_by_id`** — it sends one Kafka message per unique trace ID (millions/min), causing a massive produce rate explosion and consumer lag.
- **`least_backup`** (v0.162.0+ franz-go) is the correct partitioner for traces — same batch count, just distributed evenly across partitions.
- **`partition_logs_by_resource_attributes`** is safe for logs — resource attribute cardinality is ~20–50 (service names), not millions.
- **Sarama nil-key behaviour**: Sarama hashes `nil` key to a fixed partition based on `murmur2(nil) % num_partitions`. After expanding otel-traces from 4 → 16 partitions, the target changed from P3 → P11, concentrating all 11 EB instances on a single partition. franz-go + least_backup eliminates this entirely.
- **Kafka lag alert thresholds** for `otel-traces` (consumer group `clickstack-otel-consumer`): Warning ≥ 500k messages, Critical ≥ 1.5M messages (~3600 seconds via `kafka_consumergroup_group_max_lag_seconds`).

### otelcol-builder version skew

Builder tool version MUST match the component versions in `otelcol-builder.yaml`. Mismatched builder (e.g., v0.124.0 builder with v0.162.0 components) compiles but fails at runtime: `otelcol.Factories.Telemetry must not be nil`. Always install the builder matching the target version before rebuilding.