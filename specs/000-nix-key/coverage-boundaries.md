# Coverage Boundaries

What is tested automatically, what is not, and how untestable areas are mitigated.

## Fully Tested in CI (every PR)

These run on every pull request to `develop` or `main` and must pass before merge.

### Host Go Tests (`test-host` job)

| Component | Test files | Flags | What's covered |
|-----------|-----------|-------|----------------|
| SSH agent protocol | `internal/agent/*_test.go` | `-race -count=1` | Message parsing, key listing, sign dispatch, socket lifecycle |
| mTLS certificates | `internal/mtls/*_test.go` | `-race -count=1` | CA/cert generation, PEM round-trip, age encryption/decryption, cert pinning, fingerprint verification, dial with mutual TLS |
| Pairing flow | `internal/pairing/*_test.go` | `-race -count=1` | QR payload encode/decode, HTTPS pairing server, token validation, cold-start idempotency, concurrent pairing rejection |
| Daemon | `internal/daemon/*_test.go` | `-race -count=1` | Device registry CRUD, control socket commands, graceful shutdown |
| Config | `internal/config/*_test.go` | `-race -count=1` | Config loading, defaults, validation |
| Errors | `internal/errors/*_test.go` | `-race -count=1` | Error hierarchy, wrapping, sentinel checks |
| Logging | `internal/logging/*_test.go` | `-race -count=1` | Structured JSON output, module field, level filtering |
| Tracing | `internal/tracing/*_test.go` | `-race -count=1` | OTEL initialization, span creation |
| Phone server (shared) | `pkg/phoneserver/*_test.go` | `-race -count=1` | gRPC server lifecycle, sign request handling, OTEL propagation |
| Protobuf | `gen/nixkey/v1/*_test.go` | `-race -count=1` | Proto round-trip, field validation |

All Go tests run with `-race` to detect data races at test time.

### Benchmarks (`test-host` job)

| Component | Test files | What's measured |
|-----------|-----------|----------------|
| mTLS handshake | `internal/mtls/bench_test.go` | TLS handshake latency per-iteration |
| Phone server | `pkg/phoneserver/bench_test.go` | gRPC sign request throughput |
| E2E sign latency | `internal/agent/latency_test.go` | Full sign path (agent -> gRPC -> response) |

Benchmark results are compared against the previous baseline; regressions >300% fail the build.

### NixOS VM Tests (`test-host` job, via `nix flake check`)

| Test | File | What's covered |
|------|------|----------------|
| Service | `nix/tests/service-test.nix` | Daemon starts, `SSH_AUTH_SOCK` responds |
| Pairing | `nix/tests/pairing-test.nix` | Full pairing flow over headscale Tailscale mesh |
| Signing | `nix/tests/signing-test.nix` | SSH sign via phone simulator over headscale |
| Adversarial | `nix/tests/adversarial-test.nix` | Rogue certs rejected, mTLS pinning enforced |
| Jaeger | `nix/tests/jaeger-test.nix` | Jaeger tracing infrastructure starts |
| Tracing E2E | `nix/tests/tracing-e2e-test.nix` | Distributed trace propagation host -> phone |

These tests use real NixOS VMs with headscale for Tailscale networking. The phone side is simulated by `test/phonesim/`.

### Lint (`lint` job)

| Tool | Scope |
|------|-------|
| golangci-lint | All Go code (includes `errcheck`, `staticcheck`, etc.) |
| nixfmt | All Nix files (rfc-style) |
| ktlint | All Kotlin code (Android) |
| RacerD (Infer) | Android code — thread-safety analysis |

### Security Scanning (`security` job)

| Scanner | Tier | What's scanned |
|---------|------|----------------|
| Trivy | 1 (required) | Filesystem vulnerability scan (CRITICAL, HIGH) |
| Semgrep | 1 (required) | SAST — Go + default rulesets |
| Gitleaks | 1 (required) | Secret detection in git history |
| govulncheck | 1 (required) | Known Go vulnerability database |
| Snyk | 1.5 (optional) | Dependency vulnerability scan |
| SonarCloud | 1.5 (optional) | Code quality + security hotspots |
| OpenSSF Scorecard | 1.5 (optional) | Supply-chain security posture (main only) |

### Android Unit Tests (`test-android` job)

| Component | Test files | What's covered |
|-----------|-----------|----------------|
| Trace context | `android/.../test/.../TraceContextTest.kt` | W3C traceparent parsing/serialization |

Runs via `./gradlew testDebugUnitTest` (JVM-based, no emulator).

## Tested on Develop Push (post-merge)

These run after CI completes on `develop` or `main` branches.

### Android Emulator E2E (`e2e.yml`)

| Test | What's covered |
|------|----------------|
| E2E helper | Full Android app lifecycle on emulator — gRPC server start, pairing flow, sign request approval |

Runs on a real Android emulator with 3 retry attempts and 60s cooldown between retries. Tests exercise the gomobile bridge, Compose UI, and gRPC server.

### Fuzz Testing (`fuzz` job in `ci.yml`)

Generative fuzzing runs 60 seconds per target on every CI run (PR + push):

| Package | Target | What's fuzzed |
|---------|--------|---------------|
| `internal/agent` | `FuzzSSHAgentProtocol` | SSH agent wire protocol parsing |
| `internal/config` | `FuzzConfigParse` | Configuration file parsing |
| `internal/pairing` | `FuzzQRPayloadParse` | QR payload deserialization |
| `internal/daemon` | `FuzzControlRequestParse` | Control socket request parsing |
| `internal/daemon` | `FuzzControlResponseParse` | Control socket response parsing |
| `internal/mtls` | `FuzzCertPEMParse` | PEM certificate parsing |
| `internal/mtls` | `FuzzCertFingerprint` | Certificate fingerprint computation |
| `gen/nixkey/v1` | `FuzzProtoSignRequest` | Protobuf SignRequest deserialization |
| `gen/nixkey/v1` | `FuzzProtoListKeysResponse` | Protobuf ListKeysResponse deserialization |
| `gen/nixkey/v1` | `FuzzProtoRoundTrip` | Protobuf marshal/unmarshal round-trip |

Crash corpus is uploaded as an artifact on failure.

## Not Tested in CI

These require real hardware or external services that cannot be replicated in CI.

| Area | Why untestable in CI | Mitigation |
|------|---------------------|------------|
| **Real Android Keystore hardware** | Requires a physical device with hardware-backed keystore (StrongBox/TEE). Emulator uses software keystore. | `KeyManager` interface tested via fakes in unit tests; software keystore exercised on emulator in E2E. |
| **Real biometric authentication** | Requires physical fingerprint sensor or face recognition hardware. | `BiometricHelper` abstracted behind interface; tests verify dialog lifecycle and callback wiring without real biometrics. |
| **Real Tailscale authentication** | Requires a real Tailscale account and auth key exchange with coordination server. | NixOS VM tests use headscale (self-hosted Tailscale control plane) for full mesh networking. `TailscaleManager` state is tested via `StateFlow` observation. |
| **Real camera QR code scanning** | Requires a physical camera to capture QR codes. | ML Kit bitmap test (`QrBitmapScanTest.kt`) generates QR codes via ZXing, renders to bitmap, and verifies ML Kit `BarcodeScanner` decodes them correctly. Covers the full decode pipeline without a camera. |
| **Real Tailscale DERP relay** | Tailscale's relay servers are external infrastructure. | Headscale in VM tests provides direct WireGuard connectivity; DERP relay behavior is not tested. |
| **Real age decryption on device boot** | Requires system keyring integration for age identity passphrase. | Age encryption/decryption is tested with deterministic fixtures (`test/fixtures/`). File permission tests verify `0600`/`0700` enforcement. |
| **Production certificate rotation** | Long-lived certificate expiry and rotation requires time manipulation. | `ExpiredCertTest.kt` tests cert expiry detection. mTLS tests generate short-lived certs for validation. |

## Per-Component Coverage Thresholds

Target thresholds for code coverage by component. Enforced via `make cover` locally; CI reports coverage in test-host artifacts.

| Component | Target | Rationale |
|-----------|--------|-----------|
| `internal/agent` | 85% | Core SSH agent protocol — high coverage essential |
| `internal/mtls` | 90% | Security-critical certificate handling |
| `internal/pairing` | 80% | Pairing flow with network I/O; some paths hard to unit-test |
| `internal/daemon` | 80% | Registry + control socket; shutdown paths tested |
| `internal/config` | 90% | Pure parsing logic, easily testable |
| `internal/errors` | 95% | Simple error types, fully testable |
| `internal/logging` | 75% | Output formatting; some paths depend on slog internals |
| `internal/tracing` | 70% | OTEL setup is mostly configuration; span creation tested |
| `pkg/phoneserver` | 80% | Shared with Android via gomobile; gRPC paths tested |
| `gen/nixkey/v1` | 70% | Generated code; fuzz tests cover deserialization |
| Android `keystore` | 70% | Hardware keystore paths use fakes; real coverage on emulator |
| Android `pairing` | 75% | QR bitmap test covers decode; network paths tested in E2E |
| Android `service` | 70% | gRPC server lifecycle tested; foreground service needs emulator |
| Android `ui` | 60% | Compose UI tested via instrumented tests; visual correctness manual |
| Android `bridge` | 70% | gomobile FFI bridge tested via `GoPhoneServerTest` |

## Summary

```
CI (every PR):
  Host Go tests with -race    -> all internal/ and pkg/ packages
  NixOS VM tests               -> service, pairing, signing, adversarial, tracing
  Lint                         -> Go, Nix, Kotlin, RacerD
  Security scan                -> Trivy, Semgrep, Gitleaks, govulncheck
  Fuzz (60s/target)            -> protocol parsers, protobuf, certs
  Android unit tests           -> JVM-only tests
  Benchmarks + regression      -> mTLS handshake, sign throughput

Post-merge (develop/main):
  Android emulator E2E         -> full app lifecycle with retry

Not in CI:
  Real hardware keystore       -> mitigated by interface fakes
  Real biometrics              -> mitigated by callback wiring tests
  Real Tailscale auth          -> mitigated by headscale in VMs
  Real camera QR               -> mitigated by ML Kit bitmap test
```
