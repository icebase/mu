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
