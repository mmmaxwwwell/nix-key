# nix-key

SSH agent that delegates private key operations to an Android phone over Tailscale with mTLS. Keys never leave the phone's hardware keystore. The host runs a daemon exposing a standard `SSH_AUTH_SOCK` Unix socket; on sign requests it dials the phone via gRPC over mTLS to get signatures.

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│ Host (NixOS)                                                     │
│                                                                  │
│  SSH client ──► SSH_AUTH_SOCK (Unix socket)                      │
│                    │                                             │
│              nix-key daemon                                      │
│              ├─ agent server (SSH agent protocol)                │
│              ├─ device registry (paired phones)                  │
│              ├─ control socket (CLI ↔ daemon IPC)                │
│              └─ gRPC client ──► Tailscale ──────────────┐        │
│                                                         │        │
│  OpenTelemetry traces ──► OTLP collector (optional)     │        │
└─────────────────────────────────────────────────────────┼────────┘
                                                          │
                                                  mTLS over Tailscale
                                                          │
┌─────────────────────────────────────────────────────────┼────────┐
│ Android Phone                                           │        │
│                                                         ▼        │
│              gRPC server (phoneserver)                            │
│              ├─ hardware keystore (keys never leave)             │
│              ├─ two-policy signing model                         │
│              │   ├─ ConfirmationPolicy (per-sign: always/never)  │
│              │   └─ UnlockPolicy (session: biometric/none)       │
│              └─ Tailscale VPN backend                            │
│                                                                  │
│  UI: Compose + Hilt                                              │
│  ├─ Pairing (QR scan → mTLS bootstrap)                          │
│  ├─ Key management (create/list/detail with unlock state)        │
│  ├─ Sign request approval (biometric dialog)                    │
│  └─ Settings (confirmation policy, unlock policy, Tailscale)    │
└──────────────────────────────────────────────────────────────────┘
```

**Wire protocol**: gRPC over mTLS (mutual TLS with certificate pinning). Protobuf service in `proto/nixkey/v1/`. OpenTelemetry W3C trace context propagated in gRPC metadata for distributed tracing.

**Pairing flow**: Host generates ephemeral HTTPS server → displays QR code with connection info → phone scans QR → mutual certificate exchange → age-encrypted certs stored on host → device registered.

**Two-policy model**: Each key on the phone has two independent security policies:
- **ConfirmationPolicy** (`Always` / `Never`) — whether each sign request requires explicit user approval (biometric or tap).
- **UnlockPolicy** (`Biometric` / `None`) — whether the key must be unlocked (via biometric) before it can be used in a session. Unlock state is managed by `KeyUnlockManager` and resets when the app is stopped.

## Quick Start

```bash
nix develop              # enter devshell with all tools
make test                # run unit + integration tests
make build               # build the nix-key binary
make validate            # test + lint + security scan
nix flake check          # full check: Go tests + NixOS VM tests + lint
```

## CLI Commands

```
nix-key daemon           # run SSH agent daemon (exposes SSH_AUTH_SOCK)
nix-key pair             # pair with a new phone (QR code flow)
nix-key devices          # list paired devices
nix-key revoke <device>  # revoke a paired device
nix-key status           # show daemon status
nix-key export <key-id>  # export SSH public key (authorized_keys format)
nix-key config           # show current configuration
nix-key logs             # tail daemon logs (human-readable)
nix-key test <device>    # test connectivity to a paired device
```

## Makefile Targets

| Target                  | Description                                        |
|-------------------------|----------------------------------------------------|
| `make dev`              | Run the binary via `go run`                        |
| `make test`             | Run unit + integration tests (with structured reporter) |
| `make test-unit`        | Unit tests only (`-short` flag)                    |
| `make test-integration` | Integration tests only (`-run Integration`)        |
| `make lint`             | Run `golangci-lint`                                |
| `make build`            | Build `nix-key` binary                             |
| `make proto`            | Generate Go code from protobuf definitions         |
| `make bench`            | Run benchmarks (mTLS handshake, phoneserver) with `-benchmem` |
| `make security-scan`    | Run Tier 1 security scanners (Trivy, Semgrep, Gitleaks, govulncheck) |
| `make validate`         | Run test + lint + security-scan (full local validation) |
| `make gomobile`         | Build gomobile AAR for Android                     |
| `make android-apk`      | Build Android debug APK (runs gomobile first)      |
| `make cover`            | Generate HTML coverage report in `coverage/`       |
| `make generate-fixtures`| Regenerate deterministic test fixtures             |
| `make clean`            | Remove binary, coverage, and test-logs             |
| `make clean-all`        | Clean + remove generated protobuf code, vendor, caches |

## Nix Commands

```bash
nix build                # build nix-key binary (x86_64-linux)
nix flake check          # run all checks: Go tests, NixOS VM tests, linting
nix develop              # enter devshell with Go, protoc, golangci-lint, etc.
```

NixOS VM tests (run via `nix flake check`):
- `service-test` — daemon starts, SSH_AUTH_SOCK responds
- `pairing-test` — full pairing flow over headscale Tailscale mesh
- `signing-test` — SSH sign via phone over headscale
- `jaeger-test` — Jaeger tracing infrastructure
- `tracing-e2e-test` — distributed trace propagation host→phone
- `adversarial-test` — rogue certificates rejected, mTLS hardening validated

## Project Structure

```
cmd/
  nix-key/              # CLI entrypoint (cobra subcommands: daemon, pair, devices,
                        #   revoke, status, export, config, logs, test)
  test-reporter/        # Structured test reporter (reads go test -json)
internal/
  agent/                # SSH agent protocol handler (Unix socket)
  config/               # Configuration loading and defaults
  daemon/               # Device registry, shutdown, control socket
  errors/               # Project error hierarchy
  logging/              # Structured JSON logger (wraps slog)
  mtls/                 # mTLS certificate management + age encryption
  pairing/              # QR code generation, HTTPS pairing server
  tracing/              # OpenTelemetry tracing initialization
pkg/
  phoneserver/          # gRPC server for phone side (shared with gomobile)
gen/
  nixkey/v1/            # Generated Go code from protobuf (+ fuzz tests)
proto/
  nixkey/v1/            # Protobuf service definitions (.proto files)
test/
  fixtures/             # Deterministic test certs, keys, age identity
  fixtures/adversarial/ # Adversarial cert fixtures (rogue CA, expired, wrong-host)
  fixtures/gen/         # Fixture generator (fixed seeds)
  phonesim/             # Phone simulator for integration/VM tests
  e2e/                  # End-to-end test helpers
test-logs/              # Structured test output (gitignored)
  unit/                 # Unit test results
  integration/          # Integration test results
  bench/                # Benchmark results
  security/             # Security scan JSON output
  ci/                   # CI-generated summaries
nix/
  package.nix           # Nix package for nix-key binary
  phonesim.nix          # Nix package for phone simulator
  module.nix            # NixOS module (systemd service, config)
  jaeger.nix            # Jaeger v2 package (fetched from GitHub releases)
  infer.nix             # Facebook Infer package (for Android static analysis)
  android-apk.nix       # Android APK build
  android-emulator.nix  # Android emulator for CI E2E tests
  tests/                # NixOS VM integration tests
android/                # Android app (Kotlin, Compose, Hilt)
  app/src/main/java/com/nixkey/
    keystore/           # Hardware keystore, biometric, sign queue, unlock manager
    pairing/            # QR payload parsing, pairing client
    service/            # gRPC server service (foreground)
    tailscale/          # Tailscale VPN backend + manager
    ui/                 # Compose UI (screens, viewmodels, navigation)
    data/               # Host repository, settings
    bridge/             # Go phoneserver bridge (gomobile)
    di/                 # Hilt dependency injection modules
    logging/            # Structured JSON logging, trace context
scripts/
  ci-summary.sh         # Aggregate test-logs into ci-summary.json
  security-scan.sh      # Local Tier 1 security scan (Trivy, Semgrep, Gitleaks, govulncheck)
  setup-branch-protection.sh  # Configure GitHub branch protection rules
  smoke-test.sh         # Local end-to-end smoke test
  verify-release-pipeline.sh  # Verify full release pipeline
specs/                  # Feature spec, plan, tasks, data model, coverage boundaries
.github/workflows/
  ci.yml                # PR CI: lint, test-host, test-android, security
  e2e.yml               # E2E: Android emulator tests (triggered after CI)
  fuzz.yml              # Fuzz: generative fuzzing on develop push (proto + agent + mtls)
  release.yml           # Release: full CI + build + SBOM + GitHub Release
```

## Testing

### Running Tests

```bash
make test               # all unit + integration tests (structured output)
make test-unit          # unit tests only (-short)
make test-integration   # integration tests only (TestIntegration*)
make bench              # benchmarks (mTLS handshake, E2E sign latency)
nix flake check         # full suite including NixOS VM tests
make cover              # HTML coverage report → coverage/index.html
```

### Test Output

All test output is piped through `cmd/test-reporter` which produces structured JSON in `test-logs/`:
- `test-logs/unit/summary.json` — unit test results
- `test-logs/integration/summary.json` — integration test results
- `test-logs/bench/latest.txt` — benchmark results
- `test-logs/security/summary.json` — security scan results
- `test-logs/ci/ci-summary.json` — aggregated CI results (produced by `scripts/ci-summary.sh`)

### Test Conventions

- TDD: write tests first, verify they fail, then implement.
- Unit tests use `-short` flag; integration tests are named `TestIntegration*`.
- Test fixtures are deterministic (fixed seeds). Regenerate with `make generate-fixtures`.
- Adversarial fixtures (`test/fixtures/adversarial/`) test rogue CA, expired cert, and wrong-host cert rejection.
- Phone simulator (`test/phonesim/`) is used in NixOS VM tests to stand in for a real Android phone.
- Fuzz tests live alongside generated code in `gen/nixkey/v1/` and in `internal/agent/`, `internal/mtls/`.

## Security Scanning

### Local Scanning

```bash
make security-scan      # run all Tier 1 scanners locally
```

Runs Trivy (vuln/secret/misconfig), Semgrep (p/golang), Gitleaks (secret detection), and govulncheck (Go vulnerability database). Each scanner produces JSON in `test-logs/security/`. Scanners not installed locally are skipped.

### CI Security (Tier 1 + Tier 1.5)

- **Tier 1** (always run, gate PRs): Trivy, Semgrep, Gitleaks, govulncheck
- **Tier 1.5** (run in CI, advisory): Snyk, SonarCloud, OpenSSF Scorecard
- Security scan results are uploaded as `security-logs` artifact

## CI/CD

### Workflows

1. **ci.yml** — runs on PRs to `develop`. Parallel jobs:
   - `lint`: golangci-lint + ktlint + nixfmt
   - `test-host`: `nix flake check` (Go tests + NixOS VM tests)
   - `test-android`: Gradle build + instrumented tests
   - `security`: Trivy, Semgrep, Gitleaks, govulncheck (Tier 1) + Snyk, SonarCloud, OpenSSF Scorecard (Tier 1.5)
   - `ci-summary`: aggregates results into `ci-summary.json` artifact

2. **e2e.yml** — triggered by `workflow_run` after CI completes on `develop`/`main`. Android emulator E2E with 3 retries and 60s cooldown. Uploads `test-logs/` + emulator logcat on failure.

3. **fuzz.yml** — triggered on push to `develop`. Runs generative fuzzing (`go test -fuzz`) on proto, agent, and mTLS packages. Each fuzz target runs in a separate invocation with `-fuzztime` limit.

4. **release.yml** — on push to `main`. Full CI + security + E2E. Builds Go binary (x86_64 + aarch64), Android APK, CycloneDX SBOM. Uses `release-please` for semantic versioning. Creates GitHub Release with all artifacts.

### Branch Protection

- `main`: requires all CI + security + E2E green
- `develop`: requires lint + test-host + test-android green
- `release-please` auto-creates release PRs on develop→main merges

### Debugging CI Failures

#### Reading `ci-summary.json`

Download the `ci-summary` artifact from the workflow run. The JSON has this structure:

```json
{
  "jobs": [
    {
      "name": "lint",
      "result": "success|failure",
      "pass": 0,
      "fail": 0,
      "skip": 0,
      "duration": "1m30s",
      "failures": []
    }
  ],
  "overall": "pass|fail"
}
```

Each job entry includes `failures` — an array of failing test names with their output. Start by checking `overall`, then drill into failing jobs.

#### Finding test-logs Artifacts

Each CI job uploads `test-logs/` as a workflow artifact on failure:
- `test-host-logs` — Go test + NixOS VM test output
- `test-android-logs` — Android test output
- `e2e-logs` — Android emulator E2E + logcat
- `security-logs` — security scan JSON output

Download from the GitHub Actions workflow run page → Artifacts section.

#### Common CI Issues

- **NixOS VM tests fail**: Check `test-host-logs` artifact. Common causes: headscale config changes (DNS, TLS, DERP), pkgs shadowing in test node functions, stale `vendorHash` in `nix/package.nix` or `nix/phonesim.nix`.
- **golangci-lint failures**: Run `make lint` locally. The `errcheck` linter catches unchecked error returns including `defer x.Close()`.
- **Nix build hash mismatch**: After changing `go.mod`/`go.sum`, update `vendorHash` in BOTH `nix/package.nix` and `nix/phonesim.nix` (they may differ).
- **Security scan failures**: Run `make security-scan` locally. Check `test-logs/security/summary.json` for which scanner found issues.
- **Fuzz failures**: Download fuzz crash artifacts from the `fuzz.yml` run. Add failing inputs to `testdata/fuzz/` corpus for regression testing.
- **CI cancellation**: `cancel-in-progress: false` is set to prevent long VM tests from being killed by new pushes.

## Coding Standards

### Go Conventions

- Follow standard Go style; run `golangci-lint` before committing.
- Use `internal/` for host-only packages, `pkg/` for code shared with Android (via gomobile).
- Errors: wrap with `fmt.Errorf("context: %w", err)`. Use the project error hierarchy in `internal/errors/` when available.
- Logging: use the structured JSON logger in `internal/logging/` (wraps `slog`). Always include a `module` field. Log to stderr.
- No global mutable state. Pass dependencies explicitly.

### Security

- All communication over mTLS with certificate pinning.
- No plaintext secrets on disk — use age encryption for cert private keys.
- SSH agent errors return `SSH_AGENT_FAILURE` with no internal details.
- File permissions: `0600` for secrets, `0700` for secret directories.

### Nix

- Host-side code is Nix-built. The flake provides devShell, packages, NixOS module, and checks.
- Format Nix files with `nixfmt` (rfc-style).
- Android app is the only non-Nix artifact.
- When creating NixOS VM tests with headscale, always apply: (1) `dns.nameservers.global` set, (2) `tls_cert_path = null`, (3) no `pkgs` in node function args.
