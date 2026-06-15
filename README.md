# Google Group Sync

Google Workspace group membership resolver via HTTP API. Fetches group memberships using the Google Admin SDK Directory API. Provides a RESTful API for querying groups by user, by group email, or listing all groups with members.

## What it does

Exposes HTTP endpoints that resolve Google Workspace group memberships. Designed to be called by other services (e.g., [zitadel-rbac-mapper](https://github.com/truvity/zitadel-rbac-mapper)) that need group information for access control decisions.

**This service has no knowledge of Zitadel** — it's a pure Google Workspace utility.

## API

### GET /users/{email}/groups — Get groups for a user

```
GET /users/user@example.com/groups
```

Response:
```json
{"email": "user@example.com", "groups": ["admins@example.com", "developers@example.com"]}
```

### GET /groups — List all groups with members

```
GET /groups
```

Response:
```json
[
  {"email": "admins@example.com", "members": ["user1@example.com", "user2@example.com"]},
  {"email": "developers@example.com", "members": ["user1@example.com"]}
]
```

### GET /groups/{email} — Get a single group's members

```
GET /groups/admins@example.com
```

Response:
```json
{"email": "admins@example.com", "members": ["user1@example.com", "user2@example.com"]}
```

### GET /groups?email=X — Legacy compat (redirects)

Redirects to `GET /users/{email}/groups` (302). Maintained for backward compatibility.

### GET /health — Health check

Returns `200 OK`.

## Features

- **Singleflight deduplication** — concurrent requests for the same email share a single in-flight Google API call
- **LRU cache with TTL** — configurable via `GGS_CACHE_TTL` (default 5m)
- **RFC 9457 Problem Details** — structured error responses

```
HTTP/1.1 400 Bad Request
Content-Type: application/problem+json

{
  "type": "https://github.com/truvity/google-group-sync/problems/invalid-email",
  "title": "Invalid Email",
  "status": 400,
  "detail": "memberKey \"not-an-email\" is not a valid email address"
}
```

```
HTTP/1.1 502 Bad Gateway
Content-Type: application/problem+json

{
  "type": "https://github.com/truvity/google-group-sync/problems/google-api-error",
  "title": "Google API Error",
  "status": 502,
  "detail": "failed to list groups for user@example.com: googleapi: Error 403: Not Authorized"
}
```

### Health check

```
GET /health → 200 OK
```

## Architecture

```
[Any caller] → POST /groups → [google-group-sync] → [Google Admin SDK] → [Google Workspace]
                                       │
                                       └─ domain-wide delegation (service account + impersonation)
```

## Deployment Modes

google-group-sync ships three binaries for three deployment scenarios:

| Mode | Binary | Entry point | Use case |
|------|--------|-------------|----------|
| **Standalone** | `google-group-sync` | `cmd/google-group-sync/` | K8s Deployment, local dev, any long-running process |
| **Lambda** | `bootstrap` | `cmd/google-group-sync-lambda/` | Standalone AWS Lambda with LWA layer |
| **Extension** | `google-group-sync` | `cmd/google-group-sync-extension/` | Lambda Extension sidecar for other Lambdas |

All three modes run the same HTTP server with the same API. The difference is lifecycle management and configuration prefix.

### Mode 1: Standalone (K8s / local)

The default binary. Runs as a regular HTTP server with signal handling.

```bash
# Local development
export GOOGLE_ADMIN_EMAIL=admin@example.com
export GOOGLE_SA_KEY_FILE=/path/to/sa-key.json
./google-group-sync

# Kubernetes via Helm chart
helm install google-group-sync oci://ghcr.io/truvity/charts/google-group-sync \
  --set env.GOOGLE_ADMIN_EMAIL=admin@example.com \
  --set env.GOOGLE_SA_KEY_FILE=/etc/secrets/sa-key.json \
  --set secrets.saKeySecretName=google-sa-key
```

The chart creates a Deployment, Service, and ServiceAccount. Mount the SA key as a Kubernetes Secret volume.

### Mode 2: Lambda (standalone function)

Runs as its own Lambda function behind [Lambda Web Adapter](https://github.com/awslabs/aws-lambda-web-adapter) (LWA). LWA translates Lambda invocation events into HTTP requests to `localhost:PORT`. The binary starts as a normal HTTP server — no Lambda SDK needed.

The Lambda binary optionally loads the SA key from AWS Secrets Manager at startup (via `SA_KEY_SECRET_NAME` env var), so the key doesn't need to be in Lambda environment variables.

```bash
aws lambda create-function \
  --function-name google-group-sync \
  --runtime provided.al2023 \
  --architectures arm64 \
  --handler bootstrap \
  --zip-file fileb://google-group-sync_lambda_v0.4.2_arm64.zip \
  --layers "arn:aws:lambda:eu-central-1:753240598075:layer:LambdaAdapterLayerArm64:24" \
  --environment "Variables={GOOGLE_ADMIN_EMAIL=admin@example.com,SA_KEY_SECRET_NAME=my/sa-key}"
```

A Pulumi example is available in `deploy/example-lambda/`.

### Mode 3: Extension (sidecar for other Lambdas)

Runs as a [Lambda Extension](https://docs.aws.amazon.com/lambda/latest/dg/lambda-extensions.html) — a sidecar process inside another Lambda's execution environment. The host Lambda calls `http://localhost:9090/groups` to resolve group memberships without needing a separate Lambda invocation.

Key differences from standalone/Lambda modes:

- **GGS_ prefix** — All env vars use the `GGS_` prefix to avoid collisions with the host Lambda's env vars (e.g., `GGS_GOOGLE_ADMIN_EMAIL`, `GGS_PORT`)
- **Port 9090** — Default port avoids conflict with the host Lambda (which typically uses 8080)
- **Health port disabled** — `GGS_HEALTH_PORT=0` by default (no separate health listener)
- **Extensions API registration** — Registers with the Lambda Extensions API at startup, calls `/next` to signal init complete
- **Secrets Manager** — Loads SA key from `GGS_SA_KEY_SECRET_NAME`

Deploy as a Lambda Layer. The extension ZIP wraps the binary in an `extensions/` directory so Lambda places it at `/opt/extensions/google-group-sync`:

```bash
# Publish as a Layer
aws lambda publish-layer-version \
  --layer-name google-group-sync-extension \
  --zip-file fileb://google-group-sync_extension_v0.4.2_arm64.zip \
  --compatible-architectures arm64

# Add to host Lambda
aws lambda update-function-configuration \
  --function-name my-webhook \
  --layers "arn:aws:lambda:...:layer:google-group-sync-extension:1" \
  --environment "Variables={...,GGS_GOOGLE_ADMIN_EMAIL=admin@example.com,GGS_SA_KEY_SECRET_NAME=my/sa-key}"
```

The host Lambda then calls `http://localhost:9090/groups` with a JSON body.

## Configuration

All configuration is via environment variables. No config files, no CLI flags beyond `--help` and `--version`.

### Standalone / Lambda mode

| Env var | Required | Default | Description |
|---------|----------|---------|-------------|
| `GOOGLE_ADMIN_EMAIL` | Yes | — | Admin email for domain-wide delegation |
| `GOOGLE_SA_KEY_JSON` | Mutual excl. | — | Raw SA key JSON (Lambda / env-based) |
| `GOOGLE_SA_KEY_FILE` | Mutual excl. | — | Path to SA key file (K8s mounted Secret) |
| `SA_KEY_SECRET_NAME` | No | — | AWS Secrets Manager secret name (Lambda only) |
| `PORT` | No | `8080` | HTTP server port |
| `HEALTH_PORT` | No | `7070` | Health probe port (separate listener) |
| `CACHE_TTL` | No | `5m` | Group membership cache TTL (Go duration) |
| `CACHE_MAX_SIZE` | No | `10000` | Max cache entries (LRU eviction) |
| `LOG_LEVEL` | No | `info` | Log level: debug, info, warn, error |
| `LOG_FORMAT` | No | `json` | Log format: json, text |

### Extension mode (GGS_ prefix)

All env vars use the `GGS_` prefix. The host Lambda's unprefixed vars are ignored.

| Env var | Required | Default | Description |
|---------|----------|---------|-------------|
| `GGS_GOOGLE_ADMIN_EMAIL` | Yes | — | Admin email for domain-wide delegation |
| `GGS_GOOGLE_SA_KEY_JSON` | Mutual excl. | — | Raw SA key JSON |
| `GGS_GOOGLE_SA_KEY_FILE` | Mutual excl. | — | Path to SA key file |
| `GGS_SA_KEY_SECRET_NAME` | No | — | AWS Secrets Manager secret name |
| `GGS_PORT` | No | `9090` | HTTP server port |
| `GGS_HEALTH_PORT` | No | `0` | Health probe port (0 = disabled) |
| `GGS_CACHE_TTL` | No | `5m` | Group membership cache TTL |
| `GGS_CACHE_MAX_SIZE` | No | `10000` | Max cache entries |
| `GGS_LOG_LEVEL` | No | `info` | Log level |
| `GGS_LOG_FORMAT` | No | `json` | Log format |

### SA key loading priority

1. `*_GOOGLE_SA_KEY_JSON` set → use directly
2. `*_SA_KEY_SECRET_NAME` set → load from Secrets Manager at startup, set `*_GOOGLE_SA_KEY_JSON`
3. `*_GOOGLE_SA_KEY_FILE` set → read from file at startup
4. None set → error at startup

`GOOGLE_SA_KEY_JSON` and `GOOGLE_SA_KEY_FILE` are mutually exclusive (error if both set).

## Authentication

The binary itself performs **no authentication**. Auth is delegated to the platform layer:

| Platform | Auth mechanism |
|----------|---------------|
| AWS Lambda | Function URL with `AWS_IAM` auth type — caller must have `lambda:InvokeFunctionUrl` permission |
| Lambda Extension | Localhost-only (no network auth needed — same execution environment) |
| Kubernetes | NetworkPolicy + service mesh — only authorized pods can reach the service |
| API Gateway | Cognito authorizer, IAM auth, or API keys at the gateway level |

This keeps the binary simple and lets each deployment model use its native auth story.

## Artifacts

Each GitHub Release publishes:

| Artifact | Format | Architecture | Description |
|----------|--------|--------------|-------------|
| `google-group-sync_<version>_linux_amd64.tar.gz` | tar.gz | amd64 | Standalone binary (Linux) |
| `google-group-sync_<version>_linux_arm64.tar.gz` | tar.gz | arm64 | Standalone binary (Linux) |
| `google-group-sync_<version>_darwin_amd64.tar.gz` | tar.gz | amd64 | Standalone binary (macOS) |
| `google-group-sync_<version>_darwin_arm64.tar.gz` | tar.gz | arm64 | Standalone binary (macOS) |
| `google-group-sync_lambda_<version>_arm64.zip` | zip | arm64 | Lambda ZIP (bootstrap binary) |
| `google-group-sync_extension_<version>_arm64.zip` | zip | arm64 | Extension ZIP (Layer) |
| `ghcr.io/truvity/google-group-sync:<version>` | OCI | multi-arch | Container image (distroless) |
| `oci://ghcr.io/truvity/charts/google-group-sync` | Helm | — | Helm chart |

## Development

```bash
devbox shell              # activate dev environment
just build                # build binary
just test                 # run unit tests
just lint                 # run linter
just test-integration     # run integration tests (requires real Google SA key)
just snapshot             # build snapshot release locally
```

### Integration Test Setup

```bash
# Store Google SA key in system keyring
secret-tool store --label='google-group-sync sa-key' \
  service google-group-sync username sa-key < /path/to/sa-key.json

# Delete the key file after storing (don't leave secrets on disk)
rm /path/to/sa-key.json

# Verify it's stored correctly
secret-tool lookup service google-group-sync username sa-key | head -c 20

# Create config (~/.config/google-group-sync/config.yaml)
# See tests/integration/main_test.go for required fields
```

## Related repos

- [truvity/zitadel-rbac-mapper](https://github.com/truvity/zitadel-rbac-mapper) — consumes this service for group-based RBAC in Zitadel
- [truvity/zitadel-operator](https://github.com/truvity/zitadel-operator) — K8s operator for Zitadel resources
- [truvity/zitadel-notify-relay](https://github.com/truvity/zitadel-notify-relay) — Zitadel notification relay

## License

MIT
