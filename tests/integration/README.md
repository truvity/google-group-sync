# Integration Tests

Integration tests run against a real Google Workspace instance. They are **not** run in CI — only locally by developers.

## Prerequisites

1. A Google Workspace domain with domain-wide delegation configured
2. A service account with `Admin SDK Directory API` readonly scope
3. A user in that workspace who belongs to at least one group

## Setup

### 1. Store the Google SA key in system keyring

```bash
# Using the testsetup helper (go-keyring)
go run ./cmd/testsetup store google-group-sync sa-key < /path/to/service-account.json

# Or using secret-tool directly (Linux, GNOME Keyring)
secret-tool store --label="google-group-sync sa-key" service google-group-sync username sa-key < /path/to/service-account.json
```

### 2. Create the config file

```bash
mkdir -p ~/.config/google-group-sync

cat > ~/.config/google-group-sync/config.yaml << 'EOF'
google:
  adminEmail: admin@example.com       # admin email for domain-wide delegation
  testEmail: user@example.com         # a user known to belong to at least one group
EOF
```

### 3. Run

```bash
go test -tags=integration -v ./tests/integration/...
```

## What the tests do

- **TestResolveGroups_KnownUser**: Resolves groups for `testEmail` and asserts at least one group is returned.
- **TestResolveGroups_UnknownUser**: Queries a nonexistent user (derived from testEmail domain) and asserts empty result or graceful error.

## Troubleshooting

- **"secret not found in keyring"**: The SA key is not stored. Run the store command above.
- **"failed to read config"**: Config file missing at `~/.config/google-group-sync/config.yaml`.
- **"googleapi: Error 403"**: The service account doesn't have domain-wide delegation enabled, or the scopes are wrong.
- **"googleapi: Error 400"**: Check that `adminEmail` is a valid admin in the workspace.
