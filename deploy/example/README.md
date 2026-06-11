# Deploy Example: AWS Lambda with Function URL

This Pulumi program deploys google-group-sync as an AWS Lambda function with:
- ARM64 (Graviton) runtime
- Lambda Web Adapter (LWA) layer for HTTP translation
- Function URL with AWS_IAM authentication
- CloudWatch Logs (JSON format, 7-day retention)

## Extension Mode (Lambda Layer)

When deployed as a Lambda Extension (sidecar for another Lambda), all configuration
env vars use the `GGS_` prefix to avoid collisions with the host Lambda:

| Extension env var       | Standard equivalent    | Default |
|-------------------------|------------------------|---------|
| `GGS_PORT`             | `PORT`                 | `9090`  |
| `GGS_HEALTH_PORT`     | `HEALTH_PORT`          | `7070`  |
| `GGS_GOOGLE_ADMIN_EMAIL` | `GOOGLE_ADMIN_EMAIL` | —       |
| `GGS_SA_KEY_SECRET_NAME` | `SA_KEY_SECRET_NAME` | —       |
| `GGS_GOOGLE_SA_KEY_JSON` | `GOOGLE_SA_KEY_JSON` | —       |
| `GGS_GOOGLE_SA_KEY_FILE` | `GOOGLE_SA_KEY_FILE` | —       |
| `GGS_CACHE_TTL`       | `CACHE_TTL`            | `5m`    |
| `GGS_CACHE_MAX_SIZE`  | `CACHE_MAX_SIZE`       | `10000` |
| `GGS_LOG_LEVEL`       | `LOG_LEVEL`            | `info`  |
| `GGS_LOG_FORMAT`      | `LOG_FORMAT`           | `json`  |

The host Lambda calls `http://localhost:9090/groups` to resolve groups.

## Prerequisites

- AWS credentials configured (`aws configure` or environment variables)
- Pulumi CLI installed (`pulumi login`)
- Google Workspace service account key JSON

## Setup

```bash
# Initialize stack
pulumi stack init dev

# Configure
pulumi config set aws:region eu-central-1
pulumi config set googleAdminEmail admin@example.com
pulumi config set --secret googleSAKeyJSON "$(cat /path/to/sa-key.json)"

# Deploy
pulumi up
```

## Outputs

- `functionName` — Lambda function name
- `functionArn` — Lambda function ARN (use for `lambda:InvokeFunction` permissions)
- `functionUrl` — Function URL (requires AWS_IAM SigV4 signing to call)

## Calling the Function

Since the Function URL uses AWS_IAM auth, callers must sign requests with SigV4:

```bash
# Using awscurl (pip install awscurl)
awscurl --service lambda \
  --region eu-central-1 \
  -X POST \
  -d '{"email": "user@example.com"}' \
  -H "Content-Type: application/json" \
  "$(pulumi stack output functionUrl)groups"
```

Or grant `lambda:InvokeFunctionUrl` to another Lambda/role and call programmatically.

## Cleanup

```bash
pulumi destroy
pulumi stack rm dev
```
