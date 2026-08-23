# Development commands for google-group-sync

# Disable go.work (parent workspace interferes with standalone module builds)
export GOWORK := "off"

# Regenerate protobuf/Connect code from proto/ into gen/.
generate:
    buf lint
    buf generate

# Format all Go files (gofmt + goimports via golangci-lint)
fmt:
    golangci-lint fmt ./...

# Build the binary
build: fmt
    go build -o bin/bootstrap ./cmd/google-group-sync/

# Run tests
test:
    go test ./... -coverprofile=coverage.out

# Run integration tests (requires real Google Workspace + keyring credentials)
test-integration:
    go test -tags=integration -v -count=1 -timeout=120s ./tests/integration/...

# Run linters
lint:
    golangci-lint run ./...

# Render the chart with the shipped values plus a fully-featured set;
# prove the schema rejects an unknown key (values.schema.json is the
# contract — a typo must fail the render, not be silently ignored).
chart-lint:
    helm lint charts/google-group-sync
    helm template google-group-sync charts/google-group-sync >/dev/null
    helm template google-group-sync charts/google-group-sync \
        --set replicaCount=1 \
        --set env.GOOGLE_ADMIN_EMAIL=admin@example.com \
        --set secrets.saKeySecretName=example-sa-key \
        --set ciliumNetworkPolicy.enabled=true \
        --set rbac.enabled=true \
        --set httpRoute.enabled=true >/dev/null
    ! helm template google-group-sync charts/google-group-sync --set bogusKey=1 >/dev/null 2>&1

# Run Go vulnerability check
vuln:
    govulncheck ./...

# Run go mod tidy
tidy:
    go mod tidy

# Clean build artifacts
clean:
    rm -rf bin/ dist/ coverage.out

# Run all checks (build + test + lint + chart-lint + vuln)
check: build test lint chart-lint vuln

# Build a snapshot release locally (no push, no tag)
snapshot:
    goreleaser release --snapshot --clean

# Package Helm chart locally
helm-package:
    helm package charts/google-group-sync --destination dist/
