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
| `MU_NODE_ID` | Required stable node identity; bound to the state file | `node-1` |
| `MU_STATE_FILE` | Private persistent state file (default `data/mu-state.json`) | `/var/lib/mu/state.json` |
| `MU_TROJAN_ADDRS` | Comma-separated list of Trojan server addresses | `trojan1.example.com,trojan2.example.com` |
| `MU_V2FLY_ADDRS` | Comma-separated list of V2Fly server addresses | `v2fly1.example.com,v2fly2.example.com` |
| `MU_SS_ADDRS` | Comma-separated list of `ss-server` management gRPC addresses | `127.0.0.1:9091` |
| `MU_SS_API_TOKEN` | Shared token for the `ss-server` management API; required when `MU_SS_ADDRS` is set | `change-me` |

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
```

## Shadowsocks 2022 multi-user server (`ss-server`)

`ss-server` is a Shadowsocks 2022 / SIP023 server that serves many users on one TCP+UDP port.
Each user has an individual key (uPSK); the server has one node key (iPSK). Clients (for example Mihomo) use the
password `SERVER_PSK:USER_PSK` with cipher `2022-blake3-aes-128-gcm` or `2022-blake3-aes-256-gcm`.
mu keeps its users in sync through the management gRPC API (`SSManagerService`) and reports their traffic.

```bash
make build-ss-server   # bin/ss-server; Dockerfile.ss-server builds an image
```

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
