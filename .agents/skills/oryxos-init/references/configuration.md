# OryxOS server configuration

## Contents

- [Purpose and boundary](#purpose-and-boundary)
- [Public configuration contract](#public-configuration-contract)
- [Load and validation sequence](#load-and-validation-sequence)
- [Defaults and validation](#defaults-and-validation)
- [Errors and redaction](#errors-and-redaction)
- [Required table tests](#required-table-tests)

## Purpose and boundary

`internal/config` owns process-level server configuration used to start the foundation HTTP
server and to bound shutdown. It does not load, represent, validate, or retain Profile business
configuration. In particular, do not add `provider`, `identity`, `tools`, `skills`,
`mcp_servers`, `notify_channels`, `schedules`, `channels`, `bootstrap`, or `settings` fields
here. Those remain the later Profile/MCP loading pipeline described in
`docs/TechnicalSolution.md` section 8.

The application supplies the configuration bytes and an environment lookup function to the
loader. Deciding which local file provides those bytes is a command/application concern; the
loader has no global working-directory lookup and must not log file contents.

## Public configuration contract

The validated runtime value is typed and contains only server settings:

```go
type ServerConfig struct {
	ListenAddress     string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

func LoadServerYAML(data []byte, lookupEnv func(string) (string, bool)) (ServerConfig, error)
```

The YAML input shape is deliberately separate from the runtime value:

```yaml
listen_address: 127.0.0.1:8080
http:
  read_header_timeout: 5s
  read_timeout: 30s
  write_timeout: 5m
  idle_timeout: 60s
shutdown_timeout: 30s
```

The implementation may use an unexported raw YAML struct with string duration fields so it can
differentiate an omitted setting (apply a default) from an explicit `0s` (reject it). It converts
only after strict decoding and validation. `ServerConfig` is immutable after `LoadServerYAML`
returns; configuration changes take effect only after process restart.

## Load and validation sequence

Perform these steps in this order:

1. Expand every exact `${NAME}` placeholder in scalar text once, using `lookupEnv`. `NAME` is
   `[A-Za-z_][A-Za-z0-9_]*`; malformed `${...}` syntax is an error at the containing field path.
   Do not recursively expand a value returned from the environment.
2. If a referenced variable is unset, fail before YAML decoding with an error that includes the
   variable name and the containing YAML field path. A set-but-empty value is expanded and then
   subject to the ordinary field validation for that field.
3. Decode the expanded document with `yaml.v3.Decoder.KnownFields(true)`. The document must
   contain exactly one YAML document; duplicate keys and trailing documents are errors.
4. Apply defaults only for omitted fields, parse durations with `time.ParseDuration`, and validate
   every field. Do not coerce malformed values or silently ignore unknown fields.
5. Return the typed `ServerConfig` or a field-path error. The error may identify a configuration
   source supplied by the caller, but must never embed raw YAML or expanded values.

Expansion occurs before strict decoding to match the system configuration pipeline. It is not a
mechanism for passing Profile credentials through server configuration.

## Defaults and validation

When the input is empty or omits a setting, use these exact defaults:

| Field path | Default | Validation |
| --- | --- | --- |
| `listen_address` | `127.0.0.1:8080` | Non-empty host:port accepted by `net.SplitHostPort`; port is an integer from 1 through 65535. |
| `http.read_header_timeout` | `5s` | Valid Go duration and greater than zero. |
| `http.read_timeout` | `30s` | Valid Go duration and greater than zero. |
| `http.write_timeout` | `5m` | Valid Go duration and greater than zero. This value intentionally accommodates synchronous Agent requests; do not replace it with a short fixed timeout. |
| `http.idle_timeout` | `60s` | Valid Go duration and greater than zero. |
| `shutdown_timeout` | `30s` | Valid Go duration and greater than zero. |

`http` may be omitted, but when present it must be a mapping. An explicit empty string, `0`,
`0s`, negative duration, malformed duration, unknown key, invalid address, or document with an
unexpected YAML type is invalid. HTTP server construction must copy all four nonzero HTTP values
to `http.Server`; application shutdown must use `ShutdownTimeout` as its deadline.

## Errors and redaction

Every caller-visible configuration error has a stable field path, for example:

```text
config http.write_timeout: must be a Go duration greater than zero
config listen_address: environment variable ORYXOS_LISTEN_ADDRESS is not set
config http: unknown field "write_timout"
```

Wrap lower-level errors with `%w` for `errors.Is`/`errors.As`, but render only the stable,
sanitized error to users and logs. An error never includes complete YAML, an expanded value, or a
credential. The redaction boundary applies even if an unexpected value was supplied in a rejected
unknown field.

Provide one shared redaction helper used by configuration errors and logging attributes. It must
replace the complete value with `[REDACTED]` when a case-insensitive field name is one of
`api_key`, `authorization`, `mcp_auth`, `mcp_token`, `password`, `secret`, `token`, `webhook_url`,
or contains one of `apikey`, `credential`, `mcp_auth`, or `webhook`. Do not attempt partial
masking. Values for those keys are not returned, formatted, or attached to `slog` records. Config code logs only the source
identifier, the field path, and the sanitized error.

## Required table tests

Use table-driven tests for `LoadServerYAML`; each row supplies YAML, an environment lookup, and
the expected typed value or sanitized failure. At minimum include these exact rows:

| Case | Input / setup | Expected assertion |
| --- | --- | --- |
| `defaults` | Empty YAML document | Exact defaults: `127.0.0.1:8080`, `5s`, `30s`, `5m`, `60s`, and `30s`. |
| `partial_defaults` | Only `listen_address: 127.0.0.1:9090` | Address changes; each timeout remains its exact default. |
| `valid_expansion` | `listen_address: ${ORYXOS_LISTEN_ADDRESS}` with lookup returning `127.0.0.1:9090` | Valid address and no placeholder remains in the result. |
| `missing_variable` | Same placeholder with lookup reporting unset | Error contains `listen_address` and `ORYXOS_LISTEN_ADDRESS`; it contains neither YAML bytes nor any unrelated environment value. |
| `invalid_duration` | `http.write_timeout: nope` | Error contains exactly the field path `http.write_timeout`; no zero/default substitution occurs. |
| `zero_or_negative_duration` | Table subcases `0s` and `-1s` for each timeout field | Each error names that field path and says it must be greater than zero. |
| `strict_unknown_field` | `http.write_timout: 5m` | Error contains `http` and `write_timout`; loading fails. |
| `invalid_address` | `listen_address: localhost` | Error contains `listen_address`; loading fails. |
| `redaction` | Rejected `webhook_url: https://example.invalid/hook/very-secret-token` and a logger/config error containing `api_key: top-secret` | Captured error/log output contains field names and `[REDACTED]` where applicable, but contains neither `very-secret-token` nor `top-secret`. |

Add focused assertions that every resulting timeout is nonzero and that the unknown top-level
Profile key `provider` is rejected rather than becoming a second configuration source.
