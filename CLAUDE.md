# nix-key

SSH agent that delegates private key operations to an Android phone over Tailscale with mTLS. Keys never leave the phone's hardware keystore. The host runs a daemon exposing a standard `SSH_AUTH_SOCK` Unix socket; on sign requests it dials the phone via gRPC over mTLS to get signatures.

## Quick Start

```bash
nix develop          # enter devshell with all tools
make test            # run unit + integration tests
make build           # build the nix-key binary
```

## Makefile Targets

| Target               | Description                                      |
|----------------------|--------------------------------------------------|
| `make dev`           | Run the binary via `go run`                      |
| `make test`          | Run unit + integration tests (with structured reporter) |
| `make test-unit`     | Unit tests only (`-short` flag)                  |
| `make test-integration` | Integration tests only (`-run Integration`)   |
| `make lint`          | Run `golangci-lint`                              |
| `make build`         | Build `nix-key` binary                           |
| `make proto`         | Generate Go code from protobuf definitions       |
| `make cover`         | Generate HTML coverage report in `coverage/`     |
| `make generate-fixtures` | Regenerate deterministic test fixtures       |
| `make clean`         | Remove binary, coverage, and test-logs           |
| `make clean-all`     | Clean + remove generated code, vendor, caches    |

## Project Structure

```
cmd/
  nix-key/           # CLI entrypoint (cobra subcommands)
  test-reporter/     # Structured test reporter (reads go test -json)
internal/
  agent/             # SSH agent protocol handler (Unix socket)
  daemon/            # Device registry, shutdown, control socket
  pairing/           # QR code generation, HTTPS pairing server
pkg/
  phoneserver/       # gRPC server for phone side (shared with gomobile)
proto/
  nixkey/v1/         # Protobuf service definitions
test/
  fixtures/          # Deterministic test certs, keys, age identity
  fixtures/gen/      # Fixture generator (fixed seeds)
test-logs/           # Structured test output (gitignored)
nix/                 # NixOS module, package, VM tests
android/             # Android app (Kotlin, Compose, Hilt)
specs/               # Feature spec, plan, tasks, data model
```

## Coding Standards

### Go Conventions

- Follow standard Go style; run `golangci-lint` before committing.
- Use `internal/` for host-only packages, `pkg/` for code shared with Android (via gomobile).
- Errors: wrap with `fmt.Errorf("context: %w", err)`. Use the project error hierarchy in `internal/errors/` when available.
- Logging: use the structured JSON logger in `internal/logging/` (wraps `slog`). Always include a `module` field. Log to stderr.
- No global mutable state. Pass dependencies explicitly.

### Testing

- TDD: write tests first, verify they fail, then implement.
- Unit tests use `-short` flag; integration tests are named `TestIntegration*`.
- Test output goes through the structured reporter to `test-logs/`.
- Test fixtures are deterministic (fixed seeds). Regenerate with `make generate-fixtures`.

### Security

- All communication over mTLS with certificate pinning.
- No plaintext secrets on disk — use age encryption for cert private keys.
- SSH agent errors return `SSH_AGENT_FAILURE` with no internal details.
- File permissions: `0600` for secrets, `0700` for secret directories.

### Nix

- Host-side code is Nix-built. The flake provides devShell, packages, NixOS module, and checks.
- Format Nix files with `nixfmt` (rfc-style).
- Android app is the only non-Nix artifact.
