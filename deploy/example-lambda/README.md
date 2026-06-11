# Deploy Example: Standalone Lambda with Function URL

Deploys google-group-sync as its own AWS Lambda function with:
- ARM64 (Graviton) runtime
- Lambda Web Adapter (LWA) layer for HTTP translation
- Function URL with AWS_IAM authentication
- SA key loaded from Secrets Manager at startup
- CloudWatch Logs (JSON format, 7-day retention)

## When to use this mode

Use standalone Lambda when google-group-sync is called by external services (via Function URL or `lambda:InvokeFunction`). The function runs independently and handles its own lifecycle.

For deploying as a sidecar inside another Lambda (Extension mode), see `deploy/example-extension/`.

## Prerequisites

- AWS credentials configured
- Pulumi CLI installed (`pulumi login`)
- Google Workspace service account key JSON

## Setup

```bash
pulumi stack init dev

pulumi config set aws:region eu-central-1
pulumi config set googleAdminEmail admin@example.com
pulumi config set --secret googleSAKeyJSON "$(cat /path/to/sa-key.json)"

pulumi up
```

## Outputs

- `functionName` — Lambda function name
- `functionArn` — Lambda function ARN
- `functionUrl` — Function URL (requires AWS_IAM SigV4 signing)
- `saKeySecretArn` — Secrets Manager secret ARN

## Calling the Function

```bash
# Using awscurl (pip install awscurl)
awscurl --service lambda \
  --region eu-central-1 \
  -X POST \
  -d '{"email": "user@example.com"}' \
  -H "Content-Type: application/json" \
  "$(pulumi stack output functionUrl)groups"
```

## Cleanup

```bash
pulumi destroy
pulumi stack rm dev
```
