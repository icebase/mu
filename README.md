# mu

## Introduction

mu is a multi-protocol user synchronization tool.

## Development

Use Go **1.27** (preferred toolchain **1.27.2**, recorded in `go.mod`). The Docker builder uses
1.27.2, and CI reads the version from `go.mod`. Linting uses **golangci-lint v2.14.0** with a v2
configuration; older golangci-lint v1 releases are not supported.

```sh
go mod download
go test -race ./...
go vet ./...
make build
golangci-lint run
golangci-lint fmt --diff
```

## Configuration

Configuration is done via environment variables. The following environment variables are supported:

| Environment Variable | Description | Example |
|----------|------|------|
| `MU_API_BASE_URL` | Base URL for the API | `https://api.example.com` |
| `MU_API_TOKEN` | API access token | `your-api-token` |
| `MU_API_DEBUG` | Include remote error details in API failure logs (default `false`) | `true` |
| `MU_NODE_ID` | Required stable node identity; bound to the state file | `node-1` |
| `MU_STATE_FILE` | Private persistent state file (default `data/mu-state.json`) | `/var/lib/mu/state.json` |
| `MU_TROJAN_ADDRS` | Comma-separated list of Trojan server addresses | `trojan1.example.com,trojan2.example.com` |
| `MU_V2FLY_ADDRS` | Comma-separated list of V2Fly server addresses (VMess inbound, tag `proxy`) | `v2fly1.example.com,v2fly2.example.com` |
| `MU_VLESS_ADDRS` | Comma-separated list of V2Fly server addresses with a VLESS inbound | `vless1.example.com,vless2.example.com` |
| `MU_VLESS_INBOUND_TAG` | Tag of the VLESS inbound on those servers (default `vless`) | `vless` |
| `MU_METRICS_ENABLED` | Serve Prometheus metrics at `/metrics` (default `true`) | `true` |
| `MU_METRICS_PORT` | Metrics listen port, all interfaces (default `9090`) | `9090` |
| `MU_TRACING_ENABLED` | Enable OpenTelemetry tracing (default `false`) | `true` |
| `MU_TRACING_EXPORTER` | `otlp` (OTLP/gRPC, default) or `stdout` (JSON spans on stderr) | `otlp` |
| `MU_TRACING_ENDPOINT` | OTLP collector `host:port` or URL; empty defers to `OTEL_EXPORTER_OTLP_*`, then `localhost:4317` | `otel-collector:4317` |
| `MU_TRACING_INSECURE` | Plaintext gRPC for `host:port` endpoints (default `true`); URLs use their scheme | `false` |
| `MU_TRACING_SERVICE_NAME` | `service.name` resource attribute (default `mu-service`) | `mu-node-1` |
| `MU_TRACING_SAMPLE_RATE` | Parent-based trace ID ratio, `0`–`1` (default `1`) | `0.1` |

### API failure diagnostics

Failed API calls emit `MU API request failed` with `rpc`, `node_id`, `http_status`,
`elapsed`, and the Twirp `code` when available. `request_id` (X-Request-ID) and
`cf_ray` (CF-Ray) are included when the server supplies them. `http_status=0`
means no HTTP response was received; `transport_failure` and `context_error`
help identify network failures and deadline/cancellation errors.

To investigate errors such as `ping MU: internal`, set `MU_API_DEBUG=true` and
restart mu (or recreate its Docker container after updating the environment).
This adds `remote_message`, intermediary `response_body`, and transport error
causes to failure logs. Each detail is limited to 2048 bytes and occurrences
of the configured API token are replaced with `[REDACTED]`. Other secrets or
user data in server responses are not automatically redacted; enable this
only while diagnosing and review logs before sharing them. Request headers,
request bodies, and arbitrary response metadata are not logged.

A Twirp `internal` code alone does not identify the underlying cause. Use the
remote message and request ID to correlate with API server logs; if the server
returns a generic message, its own logs are needed for further details.

### Configuration Example

```bash
# Set API information
export MU_API_BASE_URL="https://api.example.com"
export MU_API_TOKEN="your-api-token"
export MU_NODE_ID="node-1"
export MU_STATE_FILE="/var/lib/mu/state.json"

# Set Trojan server addresses
export MU_TROJAN_ADDRS="trojan1.example.com,trojan2.example.com"

# Set V2Fly server addresses
export MU_V2FLY_ADDRS="v2fly1.example.com,v2fly2.example.com"

# Set V2Fly servers hosting a VLESS inbound (optional)
export MU_VLESS_ADDRS="vless1.example.com"
export MU_VLESS_INBOUND_TAG="vless"
```

## Observability

mu exposes Prometheus metrics and, optionally, OpenTelemetry traces. Metrics are always on unless
`MU_METRICS_ENABLED=false`; tracing is off by default and costs nothing while disabled. The metrics
endpoint is unauthenticated and listens on all interfaces: keep it on a trusted network.

### Metrics

Scrape `http://<host>:9090/metrics`. With an OpenTelemetry Collector, use the `prometheus` receiver
(see below) to forward them over OTLP.

| Metric | Type | Labels | Use |
|--------|------|--------|-----|
| `mu_build_info` | gauge | `version`, `go_version` | Which build is running |
| `mu_api_requests_total` | counter | `rpc`, `status`, `code` | MU API outcomes per RPC; `code` is the Twirp error code (transport failures appear as `internal`) |
| `mu_api_request_duration_seconds` | histogram | `rpc` | MU API latency per RPC |
| `mu_ping_total`, `mu_ping_duration_seconds` | counter, histogram | `status` | Ping job |
| `mu_user_sync_total`, `mu_user_sync_duration_seconds` | counter, histogram | `status`, `sync_type` | Per-protocol user sync (`trojan`, `v2fly`, `vless`) |
| `mu_traffic_sync_total`, `mu_traffic_sync_duration_seconds` | counter, histogram | `status` | Traffic collection + upload cycle |
| `mu_job_last_success_timestamp_seconds` | gauge | `job` (`ping`, `sync_user`, `sync_traffic`) | Staleness alerts |
| `mu_traffic_outbox_batches`, `mu_traffic_outbox_logs` | gauge | – | Durable upload queue size; keeps growing while the API is unreachable |
| `mu_backend_user_operations_total` | counter | `sync_type`, `operation` (`add`, `remove`), `status` | Add/remove calls against backends (V2Ray adds are idempotent and repeat every sync) |
| `mu_traffic_logs_collected`, `mu_traffic_logs_uploaded` | gauge | `sync_type` / – | Records in the last cycle |
| `mu_total_upload_bytes`, `mu_total_download_bytes` | counter | `sync_type` | Reported traffic (VLESS is counted under `v2fly`) |
| `mu_users_count` | gauge | `node_id` | Users in the last snapshot |
| `mu_errors_total` | counter | `operation`, `error_type` | Failures by stage |

Example alerts: `time() - mu_job_last_success_timestamp_seconds{job="sync_traffic"} > 300` and
`mu_traffic_outbox_logs > 0` for longer than a few cycles.

### Traces

Every 60-second cycle produces three traces (`App.ping`, `App.syncTraffic`, `App.syncUser`):

- `App.ping` → `MU.Ping`
- `App.syncTraffic` → `App.flushTraffic` (attributes `traffic.batches_uploaded`, `traffic.logs_uploaded`) →
  `MU.UploadTrafficLog`, then `MU.GetUsers` and one `App.syncTraffic.<type>` span per backend
  (`traffic.logs_collected`); the root span carries `traffic.outbox_batches` / `traffic.outbox_logs`.
- `App.syncUser` (`mu.users`) → `MU.GetUsers` and one `App.syncUser.<type>` span per backend with
  `sync.users_desired`, `sync.users_enabled`, `sync.users_removed`, `sync.users_applied`
  (V2Fly/VLESS also `sync.users_observed` and `sync.stats_available`).

`MU.*` spans are client spans with `rpc.system=twirp`, `rpc.method`, `mu.node_id`,
`http.response.status_code`, `rpc.twirp.error_code`, `mu.request_id` and `mu.cf_ray` when available.
Failures set the span status to error; only the sanitized error category is recorded, never remote
messages or the API token. The W3C `traceparent` header is sent to the MU API, so API-side traces can
join mu's. Resource attributes: `service.name`, `service.version`, `mu.node_id`, host and process
information; `OTEL_RESOURCE_ATTRIBUTES` and `OTEL_EXPORTER_OTLP_*` (headers, TLS, timeout, ...) are honored.
`MU API request failed` logs include `trace_id` and `span_id` when tracing is on.

Debug without a collector (no Docker needed):

```bash
MU_TRACING_ENABLED=true MU_TRACING_EXPORTER=stdout ./bin/mu 2> mu.log
```

Send traces to a collector:

```bash
export MU_TRACING_ENABLED=true
export MU_TRACING_ENDPOINT=otel-collector:4317   # or https://collector.example.com:4317
export MU_TRACING_SAMPLE_RATE=0.25
```

Minimal Collector configuration receiving OTLP traces and scraping mu's metrics:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
  prometheus:
    config:
      scrape_configs:
        - job_name: mu
          scrape_interval: 30s
          static_configs:
            - targets: ["mu:9090"]
exporters:
  debug:
    verbosity: detailed
service:
  pipelines:
    traces:  { receivers: [otlp], exporters: [debug] }
    metrics: { receivers: [prometheus], exporters: [debug] }
```

## Docker Compose

You can use Docker Compose to run mu in a containerized environment. Here's an example `docker-compose.yml` file:

```yaml
version: '3.8'

services:
  mu:
    image: ghcr.io/icebase/mu
    container_name: mu
    restart: unless-stopped
    environment:
      - MU_API_BASE_URL=https://api.example.com
      - MU_API_TOKEN=your-api-token
      - MU_NODE_ID=node-1
      - MU_STATE_FILE=/var/lib/mu/state.json
      - MU_TROJAN_ADDRS=trojan1.example.com,trojan2.example.com
      - MU_V2FLY_ADDRS=v2fly1.example.com,v2fly2.example.com
      - MU_VLESS_ADDRS=vless1.example.com
    volumes:
      - mu-state:/var/lib/mu
    networks:
      - mu-network

volumes:
  mu-state:

networks:
  mu-network:
    driver: bridge
```

### Running with Docker Compose

1. Create a `docker-compose.yml` file with the configuration above
2. Update the environment variables with your actual values
3. Run the container:

```bash
# Start the service
docker-compose up -d

# View logs
docker-compose logs -f mu

# Stop the service
docker-compose down
```

## Synchronization, persistence, and upgrade notes

MU expects `GetUsers` to return a complete desired snapshot, not an incremental list.
Malformed records, duplicate UUIDs, duplicate case-insensitive V2Ray emails, and invalid VMess
identities reject the affected snapshot before authorization changes. `enable` must be `0` or `1`.
MU must be the sole user manager for each configured Trojan instance and V2Ray `proxy` inbound.
The APIs use plaintext gRPC: keep them on a trusted private network and do not expose them publicly.

### VLESS support

`MU_VLESS_ADDRS` manages a **VLESS** inbound (tag `MU_VLESS_INBOUND_TAG`, default `vless`) on a
v2fly (`v2ray-core` v5) instance through the same gRPC `HandlerService`/`StatsService` API as V2Fly.
Xray is not supported. Users are the same `GetUsers` snapshot and the same UUID as for VMess;
`alter_id` is ignored and the account uses empty flow and `none` encryption.

- **Backend email suffix.** V2Ray statistics are keyed by email only, so VLESS users are installed
  with the backend email `<email>#vless`. This keeps VLESS and VMess counters of one V2Ray instance
  disjoint, so a VMess and a VLESS inbound can share an instance without double billing. Each syncer
  ignores the other protocol's counters. Emails ending in `#vless` (case-insensitive) are reserved and
  reject the whole snapshot for the V2Fly and VLESS backends.
- **Inbound requirements.** The VLESS inbound must have its own tag (do not reuse `proxy`), MU must be
  its sole user manager, and it must start with an empty user list, as for VMess. Enable per-user
  statistics (`statsUserUplink`/`statsUserDownlink`) for the user's policy level.
- **Traffic type.** VLESS traffic is uploaded as `TRAFFIC_TYPE_V2FLY`; use the `vless` sync label in
  metrics to tell it apart. State is kept under a separate `vless:<addr>:<tag>` key.

### Persistent state is required

- Put `MU_STATE_FILE` on a persistent volume. Its parent must be writable by the MU process.
  State includes authentication material: MU creates private files (`0600`) and new directories (`0700`).
  Do not commit, print, or upload the file. Protect backups in the same way as credentials.
- The state stores per-backend user identities, applied V2Ray configurations, cumulative traffic
  cursors, and an upload outbox. Keep backend addresses stable; changing an address changes its state key.
- The file is bound to `MU_NODE_ID`. A different node ID, corrupt state, unsupported version,
  concurrent writer, or unwritable storage fails initialization rather than resetting history.
  File locking and atomic replacement require a POSIX local filesystem; network filesystem semantics
  are not guaranteed. Only one process may use a state file, and different state files do not coordinate
  multiple controllers of the same backend.
- Counter cursors and pending logs are committed together using a temporary file, file `fsync`, atomic
  rename, and directory `fsync`. Storage failures are reported and stop the affected reconciliation
  before additional authorization changes. Never delete the state file to clear an upload error.
- The outbox is bounded to 100,000 log records; the full state is bounded to 64 MiB. At capacity,
  collection fails without advancing cursors. Restore uploads or storage rather than discarding data.
- Retired identities are retained so remaining counter increments can still be attributed. Account
  deletion captures a final available snapshot before removal; V2Ray late traffic on surviving counters
  is collected using remembered identities. Do not prune this history while old sessions can still run.

### Traffic delivery guarantees and limits

Traffic is read without resetting backend counters. Deltas are queued before upload and retried after
failures or MU restarts, with the same batch timestamp and explicit Trojan/V2Ray traffic type.
Uploads already queued are retried even when fetching the next user list fails.

Delivery is **at least once, not exactly once**. The current MU upload API has no idempotency key.
If the server accepts a batch but its response is lost, or saving the local acknowledgement fails,
retrying can bill the same batch twice. Exactly-once billing requires a server-side idempotent batch
contract; `UploadAt` alone is not a unique batch identity.

A counter decrease is treated as an observed reset and the new value is counted. V2Ray directions
proved absent by a complete snapshot are seeded at zero before being recreated. Traffic generated
and destroyed entirely between samples cannot be recovered, and a restart/reset whose counters have
already exceeded the previous values before any observation cannot always be detected. Avoid other
consumers resetting the counters. Traffic belonging to an unknown historical identity cannot be
assigned a reliable MU user ID.

### First upgrade and recovery

1. Back up the existing deployment configuration; provision the persistent volume before starting
   this version. Do not run old and new MU processes against the same backends concurrently.
2. On the first observation of a pre-existing account without a saved cursor, MU establishes a baseline
   instead of rebilling its lifetime traffic. The interval before that first observation cannot be
   reconstructed safely. Newly added accounts are seeded at zero, so their first interval is counted.
3. Trojan-Go lists password hashes, not cleartext passwords. MU matches `SHA224(UUID)` and removes
   accounts by hash. Unknown hashes can be revoked, but their historical billing identity is unavailable.
4. V2Ray statistics are **not** an authorization roster. MU persists every account before attempting
   to install it and uses that history plus explicit disabled users to revoke accounts without counters.
   A never-recorded orphan with no statistics cannot be enumerated by this API. Before first rollout,
   reconcile the inbound configuration. On every V2Ray start, use a known-clean (preferably empty)
   MU-managed inbound user configuration; do not reintroduce stale static users.
5. Pre-existing V2Ray accounts with unknown applied configuration are replaced once to establish the
   desired credentials/policy. This can create a brief authentication gap; plan the initial migration
   accordingly. Subsequent unchanged syncs do not remove/recreate the user. Credentials, email, level,
   and alter ID changes are reconciled explicitly. This API cannot verify the credentials of an
   existing remote email; external modifications under the same email are unsupported. Keep MU the
   sole controller and preserve the clean-start configuration requirement above.
6. Preserve the volume across upgrades and restore state together with the matching node/backend
   identities. Restoring an old backup can replay acknowledged traffic or obsolete cursors; reconcile
   billing before resuming. Do not downgrade to a version that resets counters without a migration plan.

The first sync runs immediately. Each RPC has a deadline, each backend gets a separate time budget,
and failures are aggregated without skipping other backends. Partial collection/change failures are
reported as failures, not successful empty syncs. SIGINT/SIGTERM cancel work and close clients/state;
queued data remains on disk for the next start. Monitor sync failure counters and provision disk space.
