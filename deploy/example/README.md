# Deploy Example: AWS Lambda with Function URL

This Pulumi program deploys google-group-sync as an AWS Lambda function with:
- ARM64 (Graviton) runtime
- Lambda Web Adapter (LWA) layer for HTTP translation
- Function URL with AWS_IAM authentication
- CloudWatch Logs (JSON format, 7-day retention)

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
