# Deploy Example: Lambda Extension (Sidecar)

Deploys google-group-sync as a Lambda Extension sidecar inside a host Lambda function.

## Architecture

```
[Host Lambda (my-webhook)]
  ├── LWA layer (event→HTTP on :8080)
  ├── google-group-sync extension layer (HTTP on :9090)
  │     └── reads GGS_SA_KEY_SECRET_NAME from Secrets Manager at INIT
  └── host binary
        ├── API on :8080 (via LWA)
        └── calls http://localhost:9090/groups
```

The host Lambda calls `http://localhost:9090/groups` to resolve Google Workspace group memberships without a separate Lambda invocation (no cold start penalty, no network hop).

## When to use this mode

Use Extension mode when:
- Multiple Lambda functions need group resolution (share one Layer)
- You want to avoid the latency of invoking a separate Lambda
- The groups resolver should share the host's execution environment

For deploying as a standalone Lambda (its own Function URL), see `deploy/example-lambda/`.

## Env var prefix: GGS_

All extension env vars use the `GGS_` prefix to avoid collisions with the host Lambda:

| Extension env var | Standard equivalent | Default |
|---|---|---|
| `GGS_GOOGLE_ADMIN_EMAIL` | `GOOGLE_ADMIN_EMAIL` | — |
| `GGS_SA_KEY_SECRET_NAME` | `SA_KEY_SECRET_NAME` | — |
| `GGS_PORT` | `PORT` | `9090` |
| `GGS_HEALTH_PORT` | `HEALTH_PORT` | `0` (disabled) |
| `GGS_CACHE_TTL` | `CACHE_TTL` | `5m` |
| `GGS_LOG_FORMAT` | `LOG_FORMAT` | `json` |

## Prerequisites

- AWS credentials configured
- Pulumi CLI installed (`pulumi login`)
- Google Workspace service account key JSON
- Host Lambda ZIP uploaded to S3

## Setup

```bash
pulumi stack init dev

pulumi config set aws:region eu-central-1
pulumi config set googleAdminEmail admin@example.com
pulumi config set --secret googleSAKeyJSON "$(cat /path/to/sa-key.json)"
pulumi config set hostLambdaS3Bucket my-artifacts-bucket
pulumi config set hostLambdaS3Key webhooks/my-webhook/v1.0.0/bootstrap.zip

pulumi up
```

## Outputs

- `functionName` — Host Lambda function name
- `functionArn` — Host Lambda function ARN
- `functionUrl` — Function URL (requires AWS_IAM SigV4 signing)
- `extensionLayerArn` — Extension Layer ARN (reuse across multiple Lambdas)
- `saKeySecretArn` — Secrets Manager secret ARN

## Timeout

The host Lambda timeout is set to 30s to account for extension cold start overhead:
- Extension registration: ~50ms
- Secrets Manager fetch: ~200-500ms
- HTTP server startup: ~50ms

A 10s timeout may cause cold start failures.

## Cleanup

```bash
pulumi destroy
pulumi stack rm dev
```
