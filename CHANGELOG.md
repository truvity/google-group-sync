# Changelog

All notable changes to google-group-sync are documented here.

## [0.12.0] — 2026-08-05

### Added
- `GET /users/{email}/groups` now reports the account's suspension
  signal: `"suspended": true` when the serving domain's directory
  reports the account suspended, read from the user's group member
  entry (`Members.Get` status — covered by the existing group-member
  scope, no new delegation). External members and probe failures read
  as not-suspended: the signal revokes access downstream and must never
  fire on absence of evidence. Consumers deciding grants can now tell
  "suspended, revoke" from the ambiguous "no groups" (INF-518).
- `GroupResolver.ResolveUser` / `CachedResolver.ResolveUserCached` —
  the groups-plus-suspension resolution behind the field; the cache
  stores and round-trips it.

## [0.9.0] — 2026-07-05

### Added
- Structured per-lookup logging for `GET /users/{email}/groups`: one log line per lookup with the user email, group count, and whether the result was served from cache — INFO normally, WARN when the lookup returns zero groups; group names are logged only at DEBUG
- `CachedResolver.ResolveGroupsCached` — resolves user groups and additionally reports whether the result came from cache

## [0.8.0] — 2026-07-04

### Changed
- Go toolchain updated to 1.26.4 (security release: CVE-2026-42504 mime quadratic complexity, plus 2 additional stdlib fixes)
- devbox packages updated (govulncheck 1.3.0→1.5.0, just 1.51.0→1.54.0, just-lsp 0.4.5→0.4.7, helm 3.20.2→4.2.2)
- golangci-lint config migrated to v2 schema (`issues.exclude-rules` → `linters.exclusions.rules`)
- Remaining Go dependencies updated to latest:
  - github.com/felixge/httpsnoop v1.0.4→v1.1.0
  - google.golang.org/grpc v1.81.1→v1.82.0

### Security
- Go 1.26.4 fixes CVE-2026-42504 (mime: quadratic complexity in WordDecoder.DecodeHeader)
- govulncheck reports no known vulnerabilities in dependency tree

## [0.7.0] — 2026-06-21

### Added
- **Cache interface** (`pkg/cache/Cache`) — abstract contract for group resolution caching with `Get(key) ([]string, bool)` and `Set(key, groups)` methods
- **MemoryCache** — the existing hashicorp LRU + TTL implementation, now a named type implementing the Cache interface
- **Future DynamoDBCache** — documented (not implemented) shared cache for Lambda-at-scale deployments
- **PodDisruptionBudget** — Helm chart template, auto-enabled when `replicaCount >= 2` (default 2)
- **CiliumNetworkPolicy** — optional Helm template (default off), restricts ingress to specified pods
- **RBAC templates** — optional ServiceAccount + Role/RoleBinding (default off)
- **HTTPRoute template** — optional Envoy Gateway route (default off, internal-only service)
- **`just test-integration`** — Justfile target for integration tests (matches zitadel-operator pattern)

### Changed
- `replicaCount` default raised from 1 to 2 (login hot path, separate failure domain)
- `CachedResolver` depends on `cache.Cache` interface instead of concrete `*cache.Cache`
- Cache selection happens in the platform main; MemoryCache is the default
- `cache.New()` preserved as backward-compatible alias for `NewMemoryCache()`

### Infrastructure
- All three entrypoints (`cmd/google-group-sync`, `cmd/google-group-sync-lambda`, `cmd/google-group-sync-extension`) confirmed wiring through `app.RunWithOptions` — no divergent assembly

## [0.6.1] — 2026-06-21

### Changed
- Aligned golangci-lint config with operator standards
- Added gocyclo linter (threshold 25)
- Added `fmt` target to Justfile (golangci-lint fmt, runs before build)
- Added GOWORK=off to Justfile, GitHub badges to README

## [0.6.0] — 2026-06-16

### Changed
- CI: determinate-nix-action@v3, Go module/build cache, devbox-update workflow
- Dependencies updated (fiber v3.3.0, google.golang.org/api v0.285.0)

## [0.5.1] — 2026-06-15

### Changed
- CI: Renovate self-hosted, GitHub Actions hardened, security workflow

## [0.5.0] — 2026-06-11

### Added
- Unified config with `caarlos0/env`, prefix support (`GGS_`), per-binary defaults
- Extension entry point: GGS_ prefix, port 9090, health disabled, Extensions API registration
- Secrets Manager SA key loading in extension mode

### Fixed
- Disable health port in extension mode (avoid port 7070 conflict)
- Return 400 for Google API client errors instead of 502

## [0.4.2] — 2026-06-11

### Fixed
- Exclude non-binary files from extension ZIP
- Replace deprecated `archives.builds` with `ids`

## [0.4.0] — 2026-06-11

### Added
- Lambda and Extension entry points (three-binary architecture)
- Pulumi Lambda deployment example (`deploy/example-lambda/`)

## [0.3.0] — 2026-06-10

### Added
- Core implementation: Google Admin SDK resolver, LRU cache, singleflight deduplication
- HTTP server with Fiber v3, RFC 9457 Problem Details
- GitHub Actions CI + GoReleaser + Helm chart
- Unit and integration tests
- devbox + Justfile + Go module scaffold

## [0.1.0] — 2026-06-10

### Added
- Initial commit, license, README
