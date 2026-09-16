# Quickstart: Verify Lesson 16 Provider Integration

This quickstart describes the verification flow after the implementation tasks are complete. It does not add `global.yaml` to the `.oryxos` workspace contract.

## 1. Confirm locked dependencies

From the repository root:

```bash
go list -m \
  github.com/cloudwego/eino \
  github.com/cloudwego/eino-ext/components/model/deepseek \
  github.com/cloudwego/eino-ext/components/model/openai \
  gorm.io/gorm \
  github.com/glebarez/sqlite
```

Expected direct versions:

```text
github.com/cloudwego/eino v0.9.19
github.com/cloudwego/eino-ext/components/model/deepseek v0.1.7
github.com/cloudwego/eino-ext/components/model/openai v0.1.13
gorm.io/gorm v1.31.2
github.com/glebarez/sqlite v1.11.0
```

Resolved on 2026-09-16 with the fixed direct versions above. The production
dependency closure for `internal/store` and `internal/provider`, inspected with
`CGO_ENABLED=0 go list -deps`, contains `modernc.org/sqlite v1.23.1` and does
not contain `github.com/mattn/go-sqlite3` or `github.com/cloudwego/eino/adk`.
There are likewise no source imports of either forbidden package.

`go mod graph` does show `mattn/go-sqlite3` in upstream modules' own test
requirements (`gorm.io/gorm` and `modernc.org/sqlite`). That metadata is not in
the OryxOS production package closure and does not affect the successful
`CGO_ENABLED=0` build. This distinction avoids claiming that an upstream module
has no test dependency while still enforcing the project's pure-Go runtime
boundary.

## 2. Run the deterministic harness

```bash
go test ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/oryxos
```

The default test run must cover:

- strict process Provider parsing and secret expansion/redaction;
- full Profile structure parsing, undeclared Provider rejection, bad-file isolation, and duplicate-name handling;
- DeepSeek/MiniMax factory routing and two same-vendor Profiles with isolated settings;
- Tool schema conversion and `WithTools` binding without Tool execution;
- successful and failed model calls, zero-value unavailable usage, and audit-before-return behavior;
- execution of the hand-maintained SQL contract against temporary pure-Go SQLite;
- the credential-free `profiles/default.yaml` template.

## 3. Prepare opt-in live smoke inputs

Provide credentials only through the environment used for the test process:

```bash
export DEEPSEEK_API_KEY='...'
export MINIMAX_API_KEY='...'
```

Use a temporary process configuration equivalent to:

```yaml
providers:
  - name: deepseek
    api_key: ${DEEPSEEK_API_KEY}
  - name: minimax
    api_key: ${MINIMAX_API_KEY}
```

The startup schema intentionally has no `base_url`. The DeepSeek factory uses its native connector's official default endpoint, while the MiniMax factory fixes the OpenAI-compatible endpoint internally; a configured `base_url` is an unknown field and fails strict loading.

Never commit the expanded values or place them in Profile YAML.

## 4. Run live Provider smoke tests

```bash
go test -tags=integration ./internal/provider/...
```

The integration harness must run one DeepSeek request and one MiniMax OpenAI-compatible request. For each Provider it verifies:

- a non-empty assistant response;
- the expected connector path;
- preserved Tool-call information when the fixture requests Tool Calling;
- one `llm_calls` row with `success=true`.

If credentials are absent, the test must skip with a clear message. If credentials are present and a Provider call fails, the test must fail and the attempted call must still leave a sanitized `success=false` audit row.

Verification on 2026-09-16 compiled the integration harness and deliberately
removed both credential variables. The DeepSeek and MiniMax subtests skipped
with their respective missing-variable messages; no live network call was made.

## 5. Inspect the schema

The integration test database should report only the lesson's `llm_calls` table plus SQLite's own internal objects at this stage. Confirm required columns using:

```sql
PRAGMA table_info(llm_calls);
PRAGMA index_list(llm_calls);
```

The schema must originate from `internal/store/migrations/001_llm_calls.sql` (kept semantically identical to `contracts/llm_calls.sql`), never from `AutoMigrate`.

## 6. Recorded deterministic verification

Run on 2026-09-16 after `go mod tidy` and `gofmt`:

```text
go test ./...                         PASS
go vet ./...                          PASS
CGO_ENABLED=0 go test ./internal/store ./internal/provider  PASS
CGO_ENABLED=0 go build ./cmd/oryxos  PASS
```
