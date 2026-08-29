## Scope and deferrals

- [ ] This change stays within the approved scope.
- [ ] Deferred foundation surfaces (Prometheus, OpenAPI, retries, circuit breakers, and business runtime) remain absent.

## Verification evidence

- [ ] `go mod tidy -diff`
- [ ] `make fmt-check`
- [ ] `go vet ./...`
- [ ] `golangci-lint run` (v2.12.2)
- [ ] `go test ./...`
- [ ] `go test -race ./...`
- [ ] `go tool gosec ./...`
- [ ] `go tool govulncheck ./...`
- [ ] `CGO_ENABLED=0 go build` to a temporary directory

## API and architecture impact

- [ ] API envelope, stable codes, request IDs, and public JSON conventions are unchanged or documented.
- [ ] Dependency direction and foundation route/CLI inventories remain valid.

## Safety and review

- [ ] No secret, expanded credential, generated binary, tool cache, or unrelated generated noise is included.
- [ ] Any lint or security exclusion is narrow, justified, and has a review condition.
