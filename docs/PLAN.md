# Google Group Sync — Design & Implementation Plan

**Linear:** INF-369 (parent: INF-363)  
**Role:** Reference implementation for the Zitadel ecosystem (patterns established here are reused by zitadel-rbac-mapper, zitadel-notify-relay)

---

## Design Decisions

### CLI

- No subcommands, no config file. Binary starts the daemon immediately.
- All configuration via environment variables.
- Bare Go `main` with `--help` and `--version` flags (no urfave/cli, no cobra).
- `signal.NotifyContext` in main — context flows from root through all components.

```bash
google-group-sync            # reads env vars, starts daemon
google-group-sync --help     # shows env var documentation
google-group-sync --version  # prints version
```

### Configuration (env-only)

| Env var | Required | Default | Description |
|---------|----------|---------|-------------|
| `GOOGLE_ADMIN_EMAIL` | Yes | — | Admin email for domain-wide delegation |
| `GOOGLE_SA_KEY_JSON` | Mutual excl. | — | Raw SA key JSON (Lambda/env-based) |
| `GOOGLE_SA_KEY_FILE` | Mutual excl. | — | Path to SA key file (K8s mounted Secret) |
| `PORT` | No | `8080` | HTTP server port |
| `HEALTH_PORT` | No | `7070` | Health probe port |
| `CACHE_TTL` | No | `5m` | Group membership cache TTL |
| `CACHE_MAX_SIZE` | No | `10000` | Max cache entries (LRU eviction) |
| `LOG_LEVEL` | No | `info` | Log level (debug/info/warn/error) |
| `LOG_FORMAT` | No | `json` | Log format (json/text) |

**SA key loading priority:**
1. `GOOGLE_SA_KEY_JSON` set → use directly (Lambda: loaded from Secrets Manager at startup)
2. `GOOGLE_SA_KEY_FILE` set → read from file (K8s: mounted Secret volume)
3. Neither → error at startup with clear message

No config files. No `--config` flag. No YAML. Service reads env vars only.

### API

**Endpoint:** `POST /groups`

**Request:**
```json
{"email": "user@example.com"}
```

**Response (success):**
```
HTTP/1.1 200 OK
Content-Type: application/json

{"groups": ["admins@example.com", "developers@example.com"]}
```

**Response (error, RFC 9457 Problem Details):**
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

**Health probe:** `GET /health` → `200 OK` (K8s readiness + Lambda Web Adapter)

**Design rules:**
- JSON only (no text/plain — use `jq` for shell pipelines)
- No GET support (POST-only query)
- `application/problem+json` for all errors (RFC 9457)

### Authentication

No authentication in the binary. Auth is delegated to the platform:

| Platform | Auth mechanism |
|----------|---------------|
| AWS Lambda | Function URL with `AWS_IAM` auth type |
| Kubernetes | NetworkPolicy — only authorized pods reach the service |
| API Gateway | Cognito authorizer, IAM auth, or API keys |

This keeps the binary simple and deployment-agnostic.

### Caching

In-memory LRU cache with TTL to reduce Google Workspace API load.

```
Request → cache lookup (email as key)
  → HIT (not expired): return cached groups immediately
  → MISS or EXPIRED: call Google API, store result, return
```

- Library: `github.com/hashicorp/golang-lru/v2` with TTL wrapper
- Group membership changes infrequently (minutes/hours) — 5min default TTL is safe
- Lambda warm invocations keep cache; cold starts reset (acceptable)
- K8s Deployment keeps cache persistently across requests

### HTTP Framework

- `fiber/v3` — lightweight, fast
- `samber/slog-fiber` — request logging middleware bridging fiber to slog

### Logging

- `slog` (stdlib structured logging)
- `samber/slog-fiber` for HTTP request logging
- Context-aware: `logger.InfoContext(ctx, ...)`
- JSON format by default (CloudWatch / log aggregation)
- `LOG_FORMAT=text` for local development

### Graceful Shutdown

- `signal.NotifyContext` in `main` (SIGTERM, SIGINT)
- Context flows from root to `app.Run(ctx)` to `server.Run(ctx, ...)`
- fiber graceful shutdown with 5s timeout on context cancellation

### Lambda Web Adapter (LWA)

LWA is an AWS-provided Lambda layer that translates Lambda invocation events (Function URL, API Gateway, ALB) into standard HTTP requests sent to `localhost:PORT`. The binary runs as a normal HTTP server — no Lambda SDK, no handler signature changes.

LWA lifecycle:
1. Lambda runtime starts the binary via the `bootstrap` script
2. LWA polls `GET /health` on `HEALTH_PORT` until it returns 200
3. Lambda event arrives → LWA converts to HTTP request → sends to `localhost:PORT`
4. Binary responds → LWA converts HTTP response back to Lambda response

This lets the same binary run unchanged in Lambda and Kubernetes.

---

## Project Structure

```
google-group-sync/
├── cmd/
│   ├── google-group-sync/           # K8s / local (existing)
│   │   └── main.go
│   ├── google-group-sync-lambda/    # Lambda function (NEW)
│   │   └── main.go
│   ├── google-group-sync-extension/ # Lambda extension (NEW)
│   │   └── main.go
│   └── testsetup/main.go            # Helper: store/retrieve secrets in system keyring
├── pkg/
│   ├── app/
│   │   └── app.go                   # Wires all components, calls server.Run(ctx)
│   ├── config/
│   │   └── config.go                # Env var loader + validation
│   ├── resolver/
│   │   ├── resolver.go              # GroupResolver interface
│   │   ├── google.go                # Google Admin SDK implementation
│   │   └── google_test.go           # Unit tests (mock Google API)
│   ├── cache/
│   │   ├── cache.go                 # LRU+TTL cache wrapping hashicorp/golang-lru/v2
│   │   └── cache_test.go            # Unit tests
│   └── server/
│       ├── server.go                # fiber/v3 app setup + graceful shutdown
│       ├── handler.go               # POST /groups handler
│       ├── handler_test.go          # Unit tests (mock resolver)
│       └── problem.go               # RFC 9457 problem+json helpers
├── tests/
│   └── integration/
│       ├── main_test.go             # TestMain (keyring → env vars, real Google API)
│       └── groups_test.go           # Integration tests (//go:build integration)
├── charts/
│   └── google-group-sync/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
│           ├── deployment.yaml
│           ├── service.yaml
│           └── serviceaccount.yaml
├── deploy/
│   └── example/
│       ├── main.go                  # Pulumi Go: Lambda + IAM + SM + Function URL (AWS_IAM)
│       └── Pulumi.yaml
├── bootstrap                        # Lambda entry script (exec ./google-group-sync)
├── .goreleaser.yaml                 # Multi-arch builds + ko + Lambda ZIP
├── .github/
│   └── workflows/
│       ├── ci.yaml                  # PR: lint + unit test + build (devbox + DeterminateNix)
│       └── release.yaml             # Tag: goreleaser release (devbox + DeterminateNix)
├── Justfile
├── devbox.json
├── .envrc
├── .editorconfig
├── .gitignore
├── .golangci.yml
├── go.mod
├── go.sum
├── LICENSE
├── README.md
└── docs/
    └── PLAN.md                      # This file
```

### Package Responsibilities

| Package | Responsibility |
|---------|---------------|
| `cmd/google-group-sync` | Entry point: parse `--help`/`--version`, `signal.NotifyContext`, call `app.Run(ctx)` |
| `pkg/app` | Wire all components: load config, create logger, resolver, cache, start server |
| `pkg/config` | Load and validate env vars, SA key mutual exclusion logic |
| `pkg/resolver` | `GroupResolver` interface + Google Admin SDK implementation |
| `pkg/cache` | LRU+TTL cache wrapping hashicorp/golang-lru/v2 |
| `pkg/server` | fiber/v3 HTTP server, routes, handlers, problem+json, graceful shutdown |
| `cmd/testsetup` | Developer helper to store/retrieve SA key in system keyring for integration tests |

---

## Multi-Binary Architecture

Three entry points from the same `pkg/` core:

| Entry point | Purpose | Binary name | AWS deps | Artifact |
|-------------|---------|-------------|----------|----------|
| `cmd/google-group-sync/` | Pure HTTP daemon (K8s, local dev) | `google-group-sync` | None | Raw binary + Docker image |
| `cmd/google-group-sync-lambda/` | Standalone Lambda function | `bootstrap` | Optional (SM for SA key) | Lambda ZIP |
| `cmd/google-group-sync-extension/` | Lambda extension (sidecar for other Lambdas) | `google-group-sync` | Minimal (Extensions API register) | Extension ZIP |

All three share `pkg/` — same core logic, different lifecycle wrappers.

**Lambda function (`cmd/google-group-sync-lambda/`):**
- Binary named `bootstrap` (Lambda runtime requirement)
- LWA layer handles event→HTTP translation
- Optionally loads SA key from Secrets Manager (`SA_KEY_SECRET_NAME` env var)

**Lambda extension (`cmd/google-group-sync-extension/`):**
- Registers with Lambda Extensions API at startup (~10 lines)
- Runs as a sidecar process inside another Lambda
- Listens on configurable port (default 9090, different from main function's 8080)
- Consumers attach as a Lambda Layer, then call `http://localhost:9090/groups`

**K8s sidecar pattern:**
- Use the Docker image as a sidecar container in another pod
- Same `http://localhost:9090/groups` pattern as Lambda extension

---

## Artifacts (published on release)

| Artifact | Source cmd/ | Binary | Use case |
|----------|------------|--------|----------|
| Raw binary (linux/darwin, amd64/arm64) | `cmd/google-group-sync/` | `google-group-sync` | Local dev, custom deployments |
| Docker image | `cmd/google-group-sync/` | `google-group-sync` | K8s standalone or sidecar |
| Helm chart (OCI) | — | — | K8s standalone deployment |
| Lambda ZIP | `cmd/google-group-sync-lambda/` | `bootstrap` | Standalone Lambda with LWA |
| Extension ZIP | `cmd/google-group-sync-extension/` | `google-group-sync` | Sidecar layer for other Lambdas |

The Lambda ZIP includes the compiled binary renamed to `bootstrap` (the Lambda runtime requirement). LWA layer is added at deployment time — it is not bundled in the ZIP.

---

## Implementation Steps

### Phase 1: Core (minimal working binary)
1. [ ] `pkg/config/config.go` — env var loader + validation (SA key mutual exclusion)
2. [ ] `pkg/resolver/resolver.go` — `GroupResolver` interface
3. [ ] `pkg/resolver/google.go` — Google Admin SDK (JWT + domain-wide delegation)
4. [ ] `pkg/cache/cache.go` — LRU+TTL cache (hashicorp/golang-lru/v2)
5. [ ] `pkg/server/problem.go` — RFC 9457 error helpers
6. [ ] `pkg/server/handler.go` — `POST /groups` handler (resolver + cache)
7. [ ] `pkg/server/server.go` — fiber/v3 app, slog-fiber middleware, health probe, graceful shutdown
8. [ ] `pkg/app/app.go` — wire config → logger → resolver → cache → server
9. [ ] `cmd/google-group-sync/main.go` — bare Go main, `--help`/`--version`, `signal.NotifyContext`, call `app.Run(ctx)`
10. [ ] Unit tests: handler (mock resolver), cache (TTL/eviction), config (env parsing)

### Phase 2: Testing
11. [ ] `cmd/testsetup/main.go` — CLI helper to store/retrieve keyring secrets (go-keyring)
12. [ ] `tests/integration/main_test.go` — TestMain (loads SA key from keyring → sets env)
13. [ ] `tests/integration/groups_test.go` — real Google API tests (`//go:build integration`)

### Phase 3: Release infrastructure
14. [ ] `bootstrap` — Lambda entry script (`#!/bin/sh exec ./google-group-sync`)
15. [ ] `.goreleaser.yaml` — multi-arch builds (binary + ko image + Lambda ZIP)
16. [ ] `.github/workflows/ci.yaml` — devbox + DeterminateNix, lint + test on PR
17. [ ] `.github/workflows/release.yaml` — devbox + DeterminateNix, goreleaser on tag push
18. [ ] `charts/google-group-sync/` — Helm chart (Deployment + Service + SA)

### Phase 4: Deployment example
19. [ ] `deploy/example/main.go` — Pulumi Go: Lambda + IAM + SM + Function URL (AWS_IAM auth, LWA layer arm64)
20. [ ] `deploy/example/Pulumi.yaml`

### Phase 5: Documentation
21. [ ] Update README.md with final env var reference and usage examples
22. [ ] Tag v0.1.0

### Phase 6: Multi-binary support
23. [ ] `cmd/google-group-sync-lambda/main.go` — Lambda entry point (optional SM key loading)
24. [ ] `cmd/google-group-sync-extension/main.go` — Extension registration + HTTP server
25. [ ] Update `.goreleaser.yaml` — three builds (server, lambda, extension) + three archives
26. [ ] Publish Extension ZIP as Lambda Layer in release workflow
27. [ ] Update Pulumi example to show extension usage

---

## Testing Strategy

### Unit tests (run in CI)

```bash
go test ./...
```

- Mock `GroupResolver` interface for handler tests
- Cache TTL/eviction tests
- Config env var parsing + validation
- Error format tests (problem+json)

### Integration tests (run locally only)

```bash
go test -tags=integration ./tests/integration/...
```

**Requires:**
- Google SA key in system keyring via `go-keyring`:
  ```bash
  go run ./cmd/testsetup store < /path/to/sa.json
  ```
- Env vars set by TestMain (loaded from keyring): `GOOGLE_ADMIN_EMAIL`, `GOOGLE_SA_KEY_JSON`
- Real Google Workspace (read-only group listing, no mutations)
- Build tag: `//go:build integration`

**Test scenarios:**
- Resolve groups for a known user → returns expected groups
- Resolve groups for unknown user → returns empty array `[]`
- Invalid email format → 400 + problem+json
- Cache hit (second call within TTL) → no Google API call
- Google API unreachable/forbidden → 502 + problem+json

---

## Deployment Patterns

### Kubernetes (Helm)

```bash
helm install google-group-sync oci://ghcr.io/truvity/charts/google-group-sync \
  --set env.GOOGLE_ADMIN_EMAIL=admin@example.com \
  --set env.GOOGLE_SA_KEY_FILE=/etc/secrets/sa-key.json \
  --set secrets.saKeySecretName=google-sa-key
```

Auth: NetworkPolicy restricts access to authorized caller pods only. No auth logic in the binary.

### AWS Lambda (with Lambda Web Adapter)

- ARM64 (Graviton) for cost efficiency
- Lambda Web Adapter (LWA) layer translates Lambda events → HTTP to localhost:PORT
- Function URL with `AWS_IAM` auth (caller must have `lambda:InvokeFunctionUrl`)
- SA key stored in AWS Secrets Manager, loaded into `GOOGLE_SA_KEY_JSON` env var
- CloudWatch Logs with JSON format
- Health check on `HEALTH_PORT` used by LWA readiness probe

The Pulumi example in `deploy/example/` demonstrates:
- Lambda function with LWA layer (arm64)
- Function URL with AWS_IAM auth type
- Secrets Manager for SA key
- IAM execution role with minimal permissions

---

## CI/CD

### GitHub Actions

Both CI and Release workflows use:
- **DeterminateNix** — Nix installer for reproducible devbox environment
- **devbox** — provides Go toolchain, golangci-lint, and goreleaser

### CI workflow (on PR)

```yaml
- uses: DeterminateSystems/nix-installer-action@v17
- uses: jetify-com/devbox-install-action@v0.14.0
- run: devbox run -- just lint
- run: devbox run -- just test
- run: devbox run -- just build
```

### Release workflow (on tag push)

```yaml
- uses: DeterminateSystems/nix-installer-action@v17
- uses: jetify-com/devbox-install-action@v0.14.0
- run: devbox run -- goreleaser release
```

Artifacts published:
- Multi-arch binaries (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64)
- Lambda ZIPs with `bootstrap` binary (linux/amd64, linux/arm64)
- ko container images pushed to `ghcr.io/truvity/google-group-sync`
- Helm chart OCI pushed to `ghcr.io/truvity/charts/google-group-sync`

---

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/gofiber/fiber/v3` | HTTP server |
| `github.com/samber/slog-fiber` | Request logging middleware (slog ↔ fiber) |
| `github.com/hashicorp/golang-lru/v2` | In-memory LRU cache |
| `golang.org/x/oauth2/google` | Google JWT auth |
| `google.golang.org/api/admin/directory/v1` | Google Admin SDK |
| `github.com/zalando/go-keyring` | System keyring (test infrastructure only) |

### Not used in this repo

| Package | Reason |
|---------|--------|
| `github.com/urfave/cli/v3` | Bare Go main is sufficient for a daemon with no subcommands |
| `github.com/gofiber/contrib/jwt` | No auth in binary — belongs in rbac-mapper/notify-relay |
| Any config file library | Env-only configuration, no YAML/TOML parsing needed |
