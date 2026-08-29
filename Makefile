.PHONY: fmt lint test test-race security build verify fmt-check

fmt:
	gofmt -w $$(find cmd internal -name '*.go')
	go tool goimports -w $$(find cmd internal -name '*.go')

fmt-check:
	set -eu; tmpdir="$$(mktemp -d)"; trap 'rm -rf "$$tmpdir"' EXIT; gofmt -d $$(find cmd internal -name '*.go') >"$$tmpdir/gofmt.diff"; test ! -s "$$tmpdir/gofmt.diff"; go tool goimports -d $$(find cmd internal -name '*.go') >"$$tmpdir/goimports.diff"; test ! -s "$$tmpdir/goimports.diff"

lint:
	golangci-lint run

test:
	go test ./...

test-race:
	go test -race ./...

security:
	go tool gosec ./cmd/... ./internal/...
	go tool govulncheck ./cmd/... ./internal/...

build:
	tmpdir="$$(mktemp -d)"; trap 'rm -rf "$$tmpdir"' EXIT; CGO_ENABLED=0 go build -o "$$tmpdir/oryxos" ./cmd/oryxos

verify:
	go mod tidy -diff
	$(MAKE) fmt-check
	go vet ./...
	$(MAKE) lint
	$(MAKE) test
	$(MAKE) test-race
	$(MAKE) security
	$(MAKE) build
