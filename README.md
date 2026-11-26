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
| `MU_SS_ADDRS` | Comma-separated list of `ss-server` management gRPC addresses | `127.0.0.1:9091` |
| `MU_SS_API_TOKEN` | Shared token for the `ss-server` management API; required when `MU_SS_ADDRS` is set | `change-me` |
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
| `mu_user_sync_total`, `mu_user_sync_duration_seconds` | counter, histogram | `status`, `sync_type` | Per-protocol user sync (`trojan`, `v2fly`, `vless`, `shadowsocks`) |
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

### V2Fly metrics

Set `MU_V2FLY_METRICS_ENABLED=true` to collect V2Fly v5 statistics on the existing
`/metrics` endpoint (default port 9090, requires `MU_METRICS_ENABLED=true`, the default).
Each scrape reads each unique address in `MU_V2FLY_ADDRS` and `MU_VLESS_ADDRS` over its
existing gRPC connection with a five-second deadline. Enable StatsService, statistics and the inbound/outbound traffic
policy flags for the directions you want to monitor.

`v2ray_up{backend}` reports success of the backend scrape. On failure, only health and
scrape duration are emitted for that backend. Runtime metrics include uptime, goroutines,
heap and GC statistics. The counters `v2ray_traffic_uplink_bytes_total` and
`v2ray_traffic_downlink_bytes_total` use `backend` (configured address), `dimension`
(inbound or outbound) and `target` (proxy tag). Select one dimension when totaling
traffic. User-level series are excluded. These counters reflect the backend process
and can reset on restart. The existing `mu_total_*_bytes` metrics track acknowledged
traffic reports.

### Report V2Fly metrics to MU Server

Set `MU_NODE_METRICS_REPORTING_ENABLED=true` to send V2Fly backend metrics to
`MU_API_BASE_URL` starting at launch and then every minute. This option is disabled by default.
Use `MU_METRICS_ENABLED=false` to run without a Prometheus endpoint. Reporting uses the
same V2Fly stats and policy settings as the scrape endpoint, plus the existing MU API token.

The MU Server must mount the generated Twirp handler for `NodeMetricsService` at
`/v1.NodeMetricsService/` (empty Twirp path prefix). It should authorize the token for the
submitted `node_id`, validate the batch and enforce uniqueness of `(node_id, batch_id)`.
A duplicate of an already accepted batch should return success. Acknowledge after persisting the
batch. Each backend sample contains a timestamp and collection status. Successful samples
also carry runtime statistics and cumulative byte counters by proxy tag.

The agent retries transient upload failures once using the same batch ID. Batches are kept
in memory for the attempt and retry, so an outage can leave gaps in the metric history.

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

## Shadowsocks 2022 multi-user server (`ss-server`)

`ss-server` is a Shadowsocks 2022 / SIP023 server that serves many users on one TCP+UDP port.
Each user has an individual key (uPSK); the server has one node key (iPSK). Clients (for example Mihomo) use the
password `SERVER_PSK:USER_PSK` with cipher `2022-blake3-aes-128-gcm` or `2022-blake3-aes-256-gcm`.
mu keeps its users in sync through the management gRPC API (`SSManagerService`) and reports their traffic.

```bash
make build-ss-server   # bin/ss-server
```

### Docker image and Compose

`ss-server` ships as its own image, `ghcr.io/icebase/mu-ss-server` (built from `Dockerfile.ss-server` by the
Docker workflow next to `ghcr.io/icebase/mu`). [`docker-compose.ss-server.yml`](docker-compose.ss-server.yml) runs both
together: mu reaches the management API at `ss-server:9091` over the compose network, so only the proxy port is published.

```bash
export SS_SERVER_PSK="$(openssl rand -base64 16)"   # 32 bytes for 2022-blake3-aes-256-gcm
export SS_API_TOKEN="$(openssl rand -hex 24)"       # used by both services
docker compose -f docker-compose.ss-server.yml up -d
```

Publish `8388` for both TCP and UDP. Do not publish `9091` (management) or `9092` (metrics) to untrusted networks.
To build locally: `docker build -f Dockerfile.ss-server -t mu-ss-server .`

| Environment Variable | Description | Default |
|----------|------|------|
| `SS_LISTEN_ADDR` | Proxy listen address (TCP and UDP) | `:8388` |
| `SS_METHOD` | `2022-blake3-aes-128-gcm` or `2022-blake3-aes-256-gcm` | `2022-blake3-aes-128-gcm` |
| `SS_SERVER_PSK` / `SS_SERVER_PSK_FILE` | Base64 iPSK (16 bytes for AES-128, 32 for AES-256), inline or from a file | required |
| `SS_API_ADDR` | Management gRPC listen address; keep it on a private interface | `127.0.0.1:9091` |
| `SS_API_TOKEN` | Token mu must send; equals mu's `MU_SS_API_TOKEN` | required |
| `SS_UDP_ENABLED` / `SS_UDP_TIMEOUT` | UDP relay and idle timeout | `true` / `5m` |
| `SS_ALLOW_PRIVATE_NETWORK` | Allow relaying to loopback, private, link-local and CGNAT addresses | `false` |
| `SS_METRICS_ENABLED` / `SS_METRICS_PORT` | Prometheus metrics | `true` / `9092` |

Generate keys with `openssl rand -base64 16` (or `32` for AES-256).

How users reach the server: the MU API returns `User.ss_user.psk` (base64 uPSK, same length as the server key).
Accounts without `ss_user` are ignored by the Shadowsocks backend, and accounts without `v2ray_user` are ignored by
Trojan and V2Fly. mu names each Shadowsocks user after `User.id`, so keys never act as identifiers.

Behavior to know about:

- Disabling or removing a user, or rotating its key, terminates that user's active TCP and UDP sessions.
- Traffic is the decrypted payload: upload is client to destination, download the reverse. Counters live in
  memory and restart at zero with the process; mu treats a counter drop as a new epoch, so traffic accrued since
  its last collection before a restart is not reported.
- After an `ss-server` restart it has no users until mu's next sync (up to 60 seconds); users are then re-installed.
- Unknown keys, replayed handshakes and forbidden destinations are dropped without reply or billing.
- Two users cannot share a key; the server rejects the second one.
- Metrics are aggregate (`ss_server_*`) to keep cardinality independent of the user count.

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
