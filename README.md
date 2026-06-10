# Google Group Sync

Google Workspace group membership resolver via HTTP API. Fetches group memberships for a given user email using the Google Admin SDK Directory API.

## What it does

Exposes a single HTTP endpoint that resolves which Google Workspace groups a user belongs to. Designed to be called by other services (e.g., [zitadel-rbac-mapper](https://github.com/truvity/zitadel-rbac-mapper)) that need group information for access control decisions.

**This service has no knowledge of Zitadel** — it's a pure Google Workspace utility.

## API

### Resolve groups

```
POST /groups
Content-Type: application/json

{"email": "user@example.com"}
```

Success response:

```
HTTP/1.1 200 OK
Content-Type: application/json

{"groups": ["admins@example.com", "developers@example.com"]}
```

Error response ([RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) Problem Details):

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

## Configuration

All configuration is via environment variables. No config files, no CLI flags beyond `--help` and `--version`.

| Env var | Required | Default | Description |
|---------|----------|---------|-------------|
| `GOOGLE_ADMIN_EMAIL` | Yes | — | Admin email for domain-wide delegation |
| `GOOGLE_SA_KEY_JSON` | Mutual excl. | — | Raw SA key JSON (Lambda / env-based) |
| `GOOGLE_SA_KEY_FILE` | Mutual excl. | — | Path to SA key file (K8s mounted Secret) |
| `PORT` | No | `8080` | HTTP server port |
| `HEALTH_PORT` | No | `7070` | Health probe port (separate listener) |
| `CACHE_TTL` | No | `5m` | Group membership cache TTL (Go duration) |
| `CACHE_MAX_SIZE` | No | `10000` | Max cache entries (LRU eviction) |
| `LOG_LEVEL` | No | `info` | Log level: debug, info, warn, error |
| `LOG_FORMAT` | No | `json` | Log format: json, text |

**SA key loading:**
- `GOOGLE_SA_KEY_JSON` set → use directly (Lambda: inline or from Secrets Manager)
- `GOOGLE_SA_KEY_FILE` set → read from file at startup (K8s: mounted Secret volume)
- Both set → error (mutually exclusive)
- Neither set → error at startup

## Authentication

The binary itself performs **no authentication**. Auth is delegated to the platform layer:

| Platform | Auth mechanism |
|----------|---------------|
| AWS Lambda | Function URL with `AWS_IAM` auth type — caller must have `lambda:InvokeFunctionUrl` permission |
| Kubernetes | NetworkPolicy + service mesh — only authorized pods can reach the service |
| API Gateway | Cognito authorizer, IAM auth, or API keys at the gateway level |

This keeps the binary simple and lets each deployment model use its native auth story.

## Deployment

### Kubernetes (Helm chart from GHCR OCI)

```bash
helm install google-group-sync oci://ghcr.io/truvity/charts/google-group-sync \
  --set env.GOOGLE_ADMIN_EMAIL=admin@example.com \
  --set env.GOOGLE_SA_KEY_FILE=/etc/secrets/sa-key.json \
  --set secrets.saKeySecretName=google-sa-key
```

The chart creates a Deployment, Service, and ServiceAccount. Mount the SA key as a Kubernetes Secret volume.

### AWS Lambda (ZIP from GitHub Release)

Download the Lambda ZIP for your architecture (arm64 recommended) from the GitHub Release assets. Deploy with the [Lambda Web Adapter](https://github.com/awslabs/aws-lambda-web-adapter) (LWA) layer.

**What is Lambda Web Adapter?**  
LWA is an AWS-provided Lambda layer that translates Lambda invocation events (API Gateway, Function URL, ALB) into standard HTTP requests sent to `localhost:PORT`. The binary runs as a normal HTTP server — no Lambda SDK, no handler signature changes. LWA starts the binary, waits for the health check to pass (`GET /health`), then proxies events as HTTP. This lets the same binary run unchanged in both Lambda and Kubernetes.

```bash
# Example: deploy with AWS CLI
aws lambda create-function \
  --function-name google-group-sync \
  --runtime provided.al2023 \
  --architectures arm64 \
  --handler bootstrap \
  --zip-file fileb://google-group-sync_v0.1.0_linux_arm64.zip \
  --layers "arn:aws:lambda:eu-central-1:753240598075:layer:LambdaAdapterLayerArm64:24" \
  --environment "Variables={GOOGLE_ADMIN_EMAIL=admin@example.com,GOOGLE_SA_KEY_JSON=...}"
```

A Pulumi example is available in `deploy/example/` showing Lambda + Function URL with AWS_IAM auth.

## Development

```bash
devbox shell              # activate dev environment
just build                # build binary
just test                 # run unit tests
just lint                 # run linter
just test-integration     # run integration tests (requires real Google SA key)
just snapshot             # build snapshot release locally
```

## Related repos

- [truvity/zitadel-rbac-mapper](https://github.com/truvity/zitadel-rbac-mapper) — consumes this service for group-based RBAC in Zitadel
- [truvity/zitadel-operator](https://github.com/truvity/zitadel-operator) — K8s operator for Zitadel resources
- [truvity/zitadel-notify-relay](https://github.com/truvity/zitadel-notify-relay) — Zitadel notification relay

## License

MIT
