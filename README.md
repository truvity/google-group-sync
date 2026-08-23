# Google Group Sync

[![CI](https://github.com/truvity/google-group-sync/actions/workflows/ci.yaml/badge.svg)](https://github.com/truvity/google-group-sync/actions/workflows/ci.yaml)
[![Release](https://github.com/truvity/google-group-sync/actions/workflows/release.yaml/badge.svg)](https://github.com/truvity/google-group-sync/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/truvity/google-group-sync)](https://goreportcard.com/report/github.com/truvity/google-group-sync)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Google Workspace group membership resolver via HTTP API. Fetches group memberships using the Google Admin SDK Directory API. Provides a RESTful API for querying groups by user, by group email, or listing all groups with members.

## What it does

Exposes HTTP endpoints that resolve Google Workspace group memberships. Designed to be called by other services (e.g., [zitadel-rbac-mapper](https://github.com/truvity/zitadel-rbac-mapper)) that need group information for access control decisions.

**This service has no knowledge of Zitadel** — it's a pure Google Workspace utility.

## API

### GET /users/{email}/groups — Get groups for a user

```json
{"email": "user@example.com", "groups": ["admins@example.com", "developers@example.com"]}
```

### GET /groups — List all groups with members

```json
[
  {"email": "admins@example.com", "members": ["user1@example.com", "user2@example.com"]},
  {"email": "developers@example.com", "members": ["user1@example.com"]}
]
```

### GET /groups/{email} — Get a single group's members

### DirectoryService (ConnectRPC)

Alongside the REST API, the service serves the `directory.v1.DirectoryService`
ConnectRPC contract (`proto/directory/v1/directory.proto`) on `CONNECT_PORT`
(default 8090; `0` disables it). Connect speaks JSON over plain HTTP POST as
well as gRPC, so `curl` and generated Go/TS clients share one endpoint. It is
the interface directory consumers use so they hold no Google credential of
their own — only this endpoint:

- `Describe` — served domains + backend, so a caller knows which addresses
  this directory vouches for.
- `Probe` — health canary against `PROBE_GROUP` (or the impersonated admin).
- `GetGroup` / `ListGroups` — flat group membership; `GetGroup` returns
  `found=false` for an absent group rather than an error (fail-safe).
- `GetAccount` / `ResolveAccounts` — an account's standing. **In-domain**
  addresses report `found`/`live` and (with the user-read scope) name;
  **out-of-domain** addresses report `in_domain=false` — *no opinion*, never
  "gone", so a consumer resolves them from their home directory.
- `ResolveUser` — groups + suspension, the grant-decision call.

`GetAccount` needs `https://www.googleapis.com/auth/admin.directory.user.readonly`
added to the service account's domain-wide delegation grant, alongside the two
group scopes. Regenerate stubs with `just generate` (buf).

### GET /health — Health check

## Architecture

```
[Any caller] → GET /users/{email}/groups → [google-group-sync] → [Google Admin SDK] → [Google Workspace]
                                                    │
                                                    └─ domain-wide delegation (service account + impersonation)
```

## Cache Interface

The resolver uses a `Cache` interface (`pkg/cache/Cache`) for group membership caching:

```go
type Cache interface {
    Get(key string) ([]string, bool)
    Set(key string, groups []string)
}
```

**Current implementation:** `MemoryCache` — in-memory LRU with TTL (hashicorp/golang-lru). Suitable for single-process deployments.

**Future implementation (not yet built):** `DynamoDBCache` — shared cache for Lambda-at-scale deployments where multiple concurrent Lambda instances benefit from a warm shared cache. Uses a DynamoDB table with TTL attribute for automatic expiration.

Cache selection happens in the platform main; default is `MemoryCache`.

## Deployment Modes

google-group-sync ships three binaries for three deployment scenarios:

| Mode | Binary | Entry point | Use case |
|------|--------|-------------|----------|
| **Standalone** | `google-group-sync` | `cmd/google-group-sync/` | K8s Deployment, local dev |
| **Lambda** | `bootstrap` | `cmd/google-group-sync-lambda/` | Standalone AWS Lambda with LWA layer |
| **Extension** | `google-group-sync` | `cmd/google-group-sync-extension/` | Lambda Extension sidecar for other Lambdas |

All three modes wire through `app.RunWithOptions` — no divergent assembly. The difference is lifecycle management, configuration prefix, and secret loading.

### Helm Chart (K8s)

```bash
helm install google-group-sync oci://ghcr.io/truvity/charts/google-group-sync \
  --set env.GOOGLE_ADMIN_EMAIL=admin@example.com \
  --set secrets.saKeySecretName=google-sa-key
```

The chart creates a Deployment (replicaCount: 2 default), Service (ClusterIP), ServiceAccount, and PodDisruptionBudget (enabled when replicas ≥ 2). It's on the login hot path as a separate failure domain.

Optional templates (default off, values-gated):
- **CiliumNetworkPolicy** — ingress only from rbac-mapper Deployment + CronJob pods
- **RBAC** — ServiceAccount + Role/RoleBinding
- **HTTPRoute** — Envoy Gateway (internal-only, rarely needed)

### Chart Values

| Key | Default | Description |
|-----|---------|-------------|
| `replicaCount` | `2` | Deployment replicas |
| `env.GOOGLE_ADMIN_EMAIL` | — | Admin email for domain-wide delegation |
| `env.PORT` | `8080` | HTTP server port |
| `env.HEALTH_PORT` | `7070` | Health probe port |
| `env.CONNECT_PORT` | `8090` | DirectoryService (ConnectRPC) port; `0` disables |
| `env.DOMAINS` | — | Served domains (comma-separated) for the DirectoryService |
| `env.CACHE_TTL` | `5m` | Group membership cache TTL |
| `env.CACHE_MAX_SIZE` | `10000` | Max cache entries (LRU) |
| `secrets.saKeySecretName` | — | K8s Secret with SA key JSON |
| `podDisruptionBudget.enabled` | `true` | PDB (auto-disabled when replicas < 2) |
| `ciliumNetworkPolicy.enabled` | `false` | CiliumNetworkPolicy |
| `rbac.enabled` | `false` | Role + RoleBinding |
| `httpRoute.enabled` | `false` | HTTPRoute (Envoy Gateway) |

## Configuration

| Env var | Required | Default | Description |
|---------|----------|---------|-------------|
| `GOOGLE_ADMIN_EMAIL` | Yes | — | Admin email for domain-wide delegation |
| `GOOGLE_SA_KEY_JSON` | Mutual excl. | — | Raw SA key JSON |
| `GOOGLE_SA_KEY_FILE` | Mutual excl. | — | Path to SA key file |
| `SA_KEY_SECRET_NAME` | No | — | AWS Secrets Manager secret name (Lambda only) |
| `PORT` | No | `8080` | HTTP server port |
| `HEALTH_PORT` | No | `7070` | Health probe port |
| `CONNECT_PORT` | No | `8090` | DirectoryService (ConnectRPC) port; `0` disables it |
| `DOMAINS` | No | — | Served domains (comma-separated) the DirectoryService vouches for |
| `PROBE_GROUP` | No | — | Group used as the DirectoryService health canary |
| `CACHE_TTL` | No | `5m` | Cache TTL (Go duration) |
| `CACHE_MAX_SIZE` | No | `10000` | Max cache entries |
| `LOG_LEVEL` | No | `info` | Log level |
| `LOG_FORMAT` | No | `json` | Log format |

Extension mode uses `GGS_` prefix (e.g., `GGS_GOOGLE_ADMIN_EMAIL`).

## Development

```bash
devbox shell          # activate dev environment
just build            # build binary
just test             # run unit tests
just lint             # run linter
just check            # build + test + lint + vuln
```

## Related

- [truvity/zitadel-rbac-mapper](https://github.com/truvity/zitadel-rbac-mapper) — consumes this service for group-based RBAC
- [truvity/zitadel-operator](https://github.com/truvity/zitadel-operator) — K8s operator for Zitadel resources

## License

MIT
