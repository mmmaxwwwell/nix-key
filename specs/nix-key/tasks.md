# Tasks: nix-key

**Approach**: TDD with fix-validate loop per phase. Full CI/CD with Tier 1 + Tier 1.5 security scanning. mTLS with age encryption at rest. gRPC wire protocol with OTEL trace propagation. NixOS VM tests with headscale for E2E. Enterprise-grade test infrastructure and documentation. Single-user, no multi-user auth.

---

## Phase 1: Test Infrastructure + Flake Setup

- [x] T001 Create `flake.nix` with devShell providing: Go 1.22+, protoc, protoc-gen-go, protoc-gen-go-grpc, age, headscale, tailscale, golangci-lint, nixfmt, gitleaks. Include `.envrc` with `use flake`. Verify `nix develop` enters shell with all tools. [FR-069, Story 6]
- [x] T002 Initialize Go module (`go mod init github.com/<owner>/nix-key`). Create `cmd/nix-key/main.go` stub with cobra subcommands skeleton. Create `Makefile` with targets: `dev`, `test`, `test-unit`, `test-integration`, `lint`, `build`, `proto`, `clean`, `clean-all`. [FR-069]
- [x] T003 Create custom structured test reporter: `test-logs/<type>/<timestamp>/summary.json` with `{pass, fail, skip, duration, failures}`. Per-failure log files in `failures/<test-name>.log` with assertion details, stack trace, and context. Wire into `make test`. [Testing infra]
- [x] T004 Create test fixtures in `test/fixtures/`: self-signed CA cert, test mTLS cert pairs (host client + phone server), test SSH Ed25519 and ECDSA keypairs, test age identity + encrypted file. All generated deterministically from fixed seeds for reproducibility. [Testing infra]
- [x] T005 Configure code coverage: `go test -coverprofile=coverage.out -covermode=atomic ./...`. Add `coverage/` to `.gitignore`. Add `make cover` target that generates HTML report. [Testing infra]
- [x] T006 Create initial `CLAUDE.md` with: project overview, quick start (`nix develop`, `make test`), available Makefile targets, project structure, coding standards (Go conventions, error wrapping, structured logging). [DX]

## Phase 2: Foundational Infrastructure

- [x] T007 Implement structured JSON logger (`internal/logging/`) wrapping Go `slog`. Fields: timestamp (ISO 8601), level, message, module, correlationId. Configurable level via config. Output to stderr. Helper to create child logger with module name. Write tests verifying JSON output format and level filtering. [FR-090, FR-091, FR-092]
- [x] T008 Implement error hierarchy (`internal/errors/`): `NixKeyError` base with `Code() string`, `Message() string`. Subclasses: `ConnectionError`, `TimeoutError`, `CertError`, `ConfigError`, `ProtocolError`. Each with unique error code prefix (e.g., `ERR_CONN_*`, `ERR_TIMEOUT_*`). Support Go `errors.Is()` and `errors.As()`. Write tests for wrapping, unwrapping, and code extraction. [FR-095, FR-096, FR-097]
- [x] T009 Implement config module (`internal/config/`): load from `~/.config/nix-key/config.json`, validate against Go struct tags + custom validation, fail-fast on startup with clear error listing all invalid fields. Three layers: hardcoded defaults → config file → env vars (with `NIXKEY_` prefix). Sensitive values (key paths) logged as "present"/"missing". Write tests: valid config loads, missing required field fails, env var overrides file, invalid type fails. [FR-098, FR-099, FR-100, FR-E06]
- [x] T010 Implement graceful shutdown (`internal/daemon/`): SIGTERM/SIGINT handler with hook registry. Shutdown sequence: log initiated → stop accepting connections → drain in-flight (with 30s deadline) → call registered hooks in reverse order → flush logs → exit. Pending sign requests are lost on restart (FR-E07 — SSH clients handle retries). Write tests verifying ordered cleanup, timeout behavior, and that in-flight requests complete before shutdown. [Graceful shutdown, FR-E07]
- [x] T011 [P] Set up Gitleaks pre-commit hook. Add `.gitleaks.toml` config. Verify hook blocks commits containing test secrets patterns. [Security]

## Phase 3: Protobuf + gRPC

- [x] T012 Write `proto/nixkey/v1/nix_key.proto`: `NixKeyAgent` service with `ListKeys`, `Sign`, `Ping` RPCs. Message types: `SSHKey` (public_key_blob, key_type, display_name, fingerprint), `SignRequest` (key_fingerprint, data, flags), `SignResponse` (signature), `PingResponse` (timestamp_ms). Add `make proto` target to generate Go code. [FR-017, FR-019]
- [x] T013 Implement `pkg/phoneserver/`: define `KeyStore` interface (`ListKeys() []SSHKey`, `Sign(fingerprint string, data []byte, flags uint32) ([]byte, error)`) and `Confirmer` interface (`RequestConfirmation(hostName, keyName, dataHash string) (bool, error)`). Implement gRPC server using these interfaces. Write tests with mock KeyStore (in-memory keys) and auto-approve Confirmer. Verify ListKeys, Sign, Ping RPCs. [FR-017, FR-018]
- [x] T014 Write gRPC integration test: start gRPC server in goroutine, connect gRPC client, exercise all three RPCs. Verify protobuf round-trip. Test error cases: Sign with unknown key → error, Sign when Confirmer denies → error. [FR-017]

## Phase 4: mTLS + Age Encryption

- [x] T015 Implement cert generation (`internal/mtls/certs.go`): generate self-signed X.509 certs with Ed25519 or ECDSA-P256 keys. Configurable expiry (default 1 year). Output PEM-encoded cert + key. Write tests verifying cert validity, expiry, and key type. [FR-010, FR-032]
- [x] T016 Implement cert pinning (`internal/mtls/`): TLS config that verifies peer cert fingerprint (SHA256) against stored value, rejecting mismatches. Create `PinnedTLSConfig(peerFingerprint string)` for both client and server modes. Write tests: matching fingerprint → success, wrong fingerprint → reject, expired cert → reject. [FR-011]
- [x] T017 Implement age encryption (`internal/mtls/age.go`): `EncryptFile(path, identityPath string)`, `DecryptToMemory(path, identityPath string) ([]byte, error)`, `GenerateIdentity(path string)`. Using `filippo.io/age`. Write tests: generate identity, encrypt file, decrypt matches original, wrong identity → error. [FR-101, FR-102, FR-104]
- [x] T018 Implement mTLS dialer and listener: `DialMTLS(addr, clientCertPath, clientKeyPath, peerFingerprint, ageIdentityPath string) (*grpc.ClientConn, error)` and `ListenMTLS(addr, serverCertPath, serverKeyPath, peerFingerprint, ageIdentityPath string) (net.Listener, error)`. Age-decrypt keys into memory before TLS config. Integration test: establish mTLS connection between two goroutines, exchange gRPC messages. [FR-010, FR-011, FR-012]

## Phase 5: SSH Agent Protocol

- [x] T019 Implement SSH agent handler (`internal/agent/`): Unix socket listener implementing `SSH2_AGENTC_REQUEST_IDENTITIES` and `SSH2_AGENTC_SIGN_REQUEST` using `golang.org/x/crypto/ssh/agent`. Accept connections on configurable `socketPath`. SSH agent responses MUST be sanitized — return SSH_AGENT_FAILURE with no internal details (FR-097). Write unit tests with mock backend verifying protocol compliance and sanitized error responses. [FR-001, FR-002, FR-097]
- [x] T020 Implement device registry (`internal/daemon/`): in-memory registry of paired devices with cert paths and connection info. Load from `devices.json` (runtime) merged with Nix-declared devices (from config). Methods: `LookupByKeyFingerprint`, `ListReachable`, `Add`, `Remove`, `UpdateLastSeen`, `UpdateIP` (updates stored Tailscale IP on successful reconnect, FR-E12). Write tests for merge logic, lookup, and IP update on reconnect. [FR-006, FR-063, FR-064, FR-E12]
- [x] T021 Wire SSH agent to gRPC: on sign request → lookup device by key fingerprint → mTLS dial to phone (MUST use configured Tailscale interface only, FR-015) → call `Sign` RPC → return signature. On list-keys → dial all reachable devices → call `ListKeys` → aggregate. Respect `allowKeyListing` (return empty without dialing if false). Implement `connectionTimeout` and `signTimeout`. Test mTLS failure logging with details (FR-E05). Test concurrent sign requests from multiple SSH clients (T-HI-08). Write integration tests with in-process gRPC phone server. [FR-004, FR-005, FR-007, FR-015, FR-E01, FR-E02, FR-E03, FR-E05]
- [x] T022 Write user-flow integration test: start agent on Unix socket, connect SSH client (`golang.org/x/crypto/ssh/agent` client), start in-process gRPC phone server, `ssh-add -L` lists keys, sign operation succeeds. Test timeout (phone server delays beyond signTimeout), denial (phone server rejects), unreachable (no phone server → fast fail). Test mid-connection drop (FR-E08): phone server closes connection mid-sign → verify SSH_AGENT_FAILURE. Test multiple phones with distinct keys (FR-029). [Story 1, SC-001, SC-006, FR-029, FR-E08]

## Phase 6: Pairing Flow

- [x] T023 Implement QR code generation (`internal/pairing/qr.go`): encode pairing payload as Base64 JSON `{v:1, host, port, cert, token, otel?}`, render QR in terminal via `skip2/go-qrcode`. Write test verifying QR decodes to valid JSON with all fields. [FR-070, FR-021]
- [x] T024 Implement temporary HTTPS pairing server (`internal/pairing/server.go`): bind to Tailscale interface, generate server cert, accept POST with `{phoneName, tailscaleIp, listenPort, serverCert, token}`. Validate token, hold connection until confirmation callback. On confirm: respond with `{hostName, hostClientCert, status:"approved"}`. On deny/timeout: respond with `{status:"denied"}` and clean up. Write test with HTTP client simulating phone. [FR-020, FR-024, FR-025, FR-026, FR-027, FR-028]
- [x] T025 Implement `nix-key pair` CLI command: generate host client cert pair, generate pairing server cert, generate one-time token, start pairing server, display QR code. On phone connection: show device info, prompt "Authorize? [y/N]". On confirm: encrypt certs with age, store device in `devices.json`, notify daemon via control socket. Generate cert pair with configured expiry. Include OTEL endpoint in QR if configured. Test: fail cleanly when Tailscale interface is unavailable (FR-E11). [FR-016, FR-031, FR-070, FR-087, FR-104, FR-E11]
- [x] T026 Implement control socket (`internal/daemon/control.go`): Unix socket at `controlSocketPath`. Line-delimited JSON protocol. Commands: `register-device`, `list-devices`, `revoke-device`, `get-status`, `get-keys`. Daemon listens, CLI commands send. Write tests for each command. [Story 5]
- [x] T027 Write pairing user-flow integration test: start daemon, run `nix-key pair` (auto-confirm via test helper), simulated phone HTTP client completes pairing. Verify: device in registry, certs encrypted with age, control socket notified daemon, `nix-key devices` shows new device. Test denied pairing: no state saved. Test token replay: rejected. [Story 2, SC-002, FR-E10]
- [x] phase6-pairing-flow-fix1 Fix phase validation failure: read specs/nix-key/validate/phase6-pairing-flow/ for failure history

## Phase 7: Android Core — Keystore + UI Scaffold [P with Phase 5-6]

- [x] T028 Set up Android project: `android/` directory, Gradle Kotlin DSL, Hilt DI, Compose Navigation, Material 3. Add dependencies: BouncyCastle, AndroidX Biometric, EncryptedSharedPreferences, Timber. Configure ktlint. Implement structured Android logging: Timber with custom JSON formatter to logcat. Log security events (pairing attempts, sign requests, failed mTLS) at INFO or above. Correlate with trace IDs when OTEL enabled. Create `UI_FLOW.md` reference task (already created in planning). [Story 3, FR-093, FR-094]
- [x] T029 Implement `KeyManager.kt`: ECDSA-P256 via native Android Keystore (`KeyGenParameterSpec` with StrongBox when available). Ed25519 via BouncyCastle + Keystore-backed AES-256 wrapping key. Methods: `createKey(name, type, policy)`, `listKeys()`, `exportPublicKey(alias) → SSH format string`, `sign(alias, data) → signature`, `deleteKey(alias)`. Write instrumented tests: create Ed25519 + ECDSA keys, export SSH format, sign data, verify non-extractability. [FR-040, FR-041, FR-042, FR-043, FR-044, FR-047]
- [x] T030 Implement Compose UI screens: ServerListScreen (paired hosts list + "Scan QR" button), KeyListScreen (keys with FAB to create), KeyDetailScreen (editable display name FR-048, type, fingerprint, confirmation policy picker, export via clipboard/share sheet/QR code FR-044, delete). SettingsScreen (deny key listing toggle FR-054, default confirmation policy, OTEL toggle + endpoint). Navigation graph with bottom nav or drawer. Write UI tests (Compose testing) for navigation, key creation flow, name editing, and export methods. [Story 3, FR-044, FR-048, FR-054, UI_FLOW.md]
- [x] T031 Implement `BiometricHelper.kt`: wrap `BiometricPrompt` API. Support per-key policy: always_ask → show prompt, biometric → BiometricPrompt with BIOMETRIC_STRONG, password → device credential, biometric_password → both, auto_approve → skip prompt (with warning on enable). Write instrumented tests with `BiometricManager` mock. [FR-045, FR-046, FR-051]
- [x] T032 Implement `SignRequestDialog.kt`: Compose dialog overlay showing host name, key name, data hash (truncated SHA256). Approve/Deny buttons. Triggers BiometricHelper per key policy. Queue for concurrent requests (show one at a time, FIFO). Write UI test for dialog display and queue behavior. [FR-050, FR-053, FR-E04]

## Phase 8: Android gRPC Server + Tailscale [P with Phase 5-6]

- [x] T033 Build gomobile bridge: compile `pkg/phoneserver` as Android AAR via gomobile. Create `GoPhoneServer.kt` Kotlin wrapper that implements `KeyStore` interface using `KeyManager.kt` and `Confirmer` interface using `BiometricHelper.kt` + `SignRequestDialog.kt`. Write integration test: Go server runs in Android process, responds to gRPC calls. [FR-017, pkg/phoneserver]
- [x] T034 Implement `TailscaleManager.kt`: initialize `libtailscale` userspace Tailscale. Auth with pre-authorized key (from settings) or interactive OAuth flow. Start/stop with app foreground/background. Get Tailscale IP. Write instrumented test verifying start/stop lifecycle (mock Tailscale backend). [FR-013, FR-013a]
- [x] T035 Implement `GrpcServerService.kt`: Android foreground service that starts gRPC server on Tailscale interface when app is foregrounded. Persistent notification "nix-key active". Bind gRPC server to Tailscale IP + configured port with mTLS (phone's server cert). Stop server + Tailscale when app backgrounds. [FR-012, FR-014]
- [x] T036 Implement pairing screen: ML Kit barcode scanner. Decode QR JSON payload. Show "Connect to hostname?" confirmation. If OTEL in payload: show "Enable tracing?" prompt. On accept: generate phone server cert (if first host) or reuse, POST to host's pairing endpoint with `{phoneName, tailscaleIp, listenPort, serverCert, token}`. Store host response (hostName, hostClientCert) in EncryptedSharedPreferences. Support multiple paired hosts (FR-030) — each host stored as separate entry in EncryptedSharedPreferences. Write instrumented test: pair with two mock hosts, verify both stored correctly. [FR-022, FR-023, FR-024, FR-026, FR-030, FR-088, Story 2]
- [x] T037 Implement TailscaleAuthScreen: first-launch screen. Text input for auth key or "Sign in with Tailscale" OAuth button. Progress indicator during Tailnet join. Success → navigate to ServerListScreen. Failure → show error with retry. Persist auth state in EncryptedSharedPreferences. [FR-013a]
- [x] T038 Wire sign request end-to-end on Android: gRPC Sign RPC received → show SignRequestDialog → user approves → BiometricHelper per policy → KeyManager.sign() → return signature via gRPC. Test denial → gRPC error. Test timeout → gRPC deadline exceeded. [Story 1, FR-050, FR-051]

## Phase 9: NixOS Module

- [x] T039 Implement `nix/module.nix`: full NixOS module under `services.nix-key` with all options from spec. Options: enable, port, tailscaleInterface, allowKeyListing (with doc note: phone can independently deny listing FR-066), signTimeout, connectionTimeout, socketPath, logLevel, tracing.otelEndpoint, tracing.jaeger.enable, secrets.ageKeyFile (FR-103), tailscale.authKeyFile (FR-013b), certExpiry, devices attrset with per-device name/tailscaleIp/port/certFingerprint/clientCert (optional, null=set by pairing FR-065)/clientKey (optional). Type checking and documentation for each option. [FR-013b, FR-060 through FR-068, FR-065, FR-066, FR-103]
- [x] T040 Implement systemd user service in module: `systemd.user.services.nix-key-agent` running `nix-key daemon`. After=network.target. Restart=on-failure. Environment from config. Create `~/.config/environment.d/50-nix-key.conf` with `SSH_AUTH_SOCK`. Create directories: `~/.config/nix-key/`, `~/.local/state/nix-key/certs/` with 0700. Write config.json from module options. [FR-061, FR-062, FR-069a]
- [x] T041 Implement `nix/package.nix`: `buildGoModule` derivation for `nix-key` binary. Pin Go version, vendor dependencies. Verify binary runs `nix-key --help`. [FR-069]
- [x] T042 Update `flake.nix` to export: `nixosModules.default` (module.nix), `packages.default` (package.nix), `checks.x86_64-linux.*` (all VM tests), `overlays.default` (adds nix-key to pkgs). Ensure non-flake import works via `default.nix`. [FR-069, FR-060]
- [x] T043 Write `nix/tests/service-test.nix`: NixOS VM test that: evaluates module with test config, verifies service starts, verifies config.json contains correct values, verifies SSH_AUTH_SOCK is set via environment.d, verifies Unix socket exists at socketPath, verifies daemon responds to control socket status query, verifies graceful shutdown (systemctl stop → daemon exits cleanly, T-NM-05/FR-E07). [T-NM-01 through T-NM-05, SC-004, SC-010, FR-E07]
- [x] T044 Write device merge test in `service-test.nix`: declare devices in Nix config, create runtime devices.json with additional device, verify daemon sees both sources merged. Test that Nix-declared device cannot be revoked via CLI (only removed by Nix rebuild). [T-NM-04, FR-063, FR-064]

## Phase 10: End-to-End Tests with Headscale

- [x] T045 Implement `test/phonesim/main.go`: full phone simulator binary using `pkg/phoneserver` + `tsnet` (Go Tailscale library — NOT libtailscale which is the Android/gomobile binding). In-memory key store (pre-loaded Ed25519 + ECDSA test keys). Auto-approve confirmer. Configurable via flags: Tailscale auth key, listen port, key list denial mode, sign delay (for timeout testing), sign denial mode. [T-E2E-00]
- [x] T046 Create `nix/phonesim.nix`: package phonesim as Nix derivation using `buildGoModule`. [T-E2E-00]
- [x] T047 Write `nix/tests/pairing-test.nix`: NixOS VM test: start headscale → create namespace + pre-auth keys → start host tailscaled (join headscale) → start phonesim tailscaled (join headscale) → verify both on Tailnet → run `nix-key pair` with auto-confirm helper → phonesim connects to pairing endpoint → verify device registered. [T-E2E-02, Story 2, SC-002]
- [x] T048 Write `nix/tests/signing-test.nix`: NixOS VM test: headscale setup + pre-paired phonesim → `ssh-add -L` lists phonesim keys → SSH sign operation with phonesim auto-approve → verify success. Test timeout: phonesim with 60s delay, signTimeout=5s → verify SSH_AGENT_FAILURE. Test denial: phonesim in deny mode → verify failure. [T-E2E-01, Story 1, SC-001, SC-006]

## Phase 11: OpenTelemetry Distributed Tracing

- [x] T049 Add OTEL to host daemon: initialize tracer provider with OTLP exporter (configurable endpoint). Create spans: `ssh-sign-request` (root), `device-lookup`, `mtls-connect`, `return-signature`. Inject W3C traceparent into gRPC metadata via `otelgrpc` interceptor. No-op tracer when otelEndpoint is null (zero overhead). Write test: perform sign request with OTEL mock collector, verify spans and parent-child relationships. [FR-080, FR-082, FR-083, FR-086]
- [x] T050 Add OTEL to phoneserver: extract traceparent from gRPC metadata via `otelgrpc` interceptor. Create child spans: `handle-sign-request`, `show-prompt`, `user-response`, `keystore-sign`. OTLP exporter configurable. Write test: gRPC call with traceparent header → verify child spans created with correct parent. [FR-081, FR-084, FR-085]
- [x] T051 Add Jaeger NixOS option: `services.nix-key.tracing.jaeger.enable` adds `services.jaeger-all-in-one` (or equivalent) and sets `otelEndpoint` to localhost:4317. Write VM test verifying Jaeger starts and accepts traces. [FR-068]
- [x] T052 Add OTEL to pairing QR: when `otelEndpoint` is configured, include it in QR payload. Phone prompts "Enable tracing?" during pairing. Store OTEL endpoint in EncryptedSharedPreferences. Phone exports traces when enabled. Write integration test: pairing with OTEL config → verify phone stores endpoint. [FR-087, FR-088, FR-023]
- [ ] T053 Write distributed trace E2E test: NixOS VM with OTEL collector (or Jaeger) + headscale + daemon + phonesim (with OTEL). Perform sign request. Query collector API. Verify: trace exists, host spans present, phone spans present, phone spans are children of host spans via traceparent. [T-E2E-03, Story 7, SC-008]

## Phase 12: CLI Polish

- [x] T054 Implement `nix-key devices`: query daemon via control socket, format as table (name, Tailscale IP, cert fingerprint, last seen, status). [FR-071]
- [x] T055 Implement `nix-key revoke <device>`: send revoke to daemon, daemon removes device from registry, deletes cert files. Confirm deletion on stdout. Reject if device is Nix-declared (print error directing user to remove from Nix config). Test that after revocation, the revoked device's cert is rejected on mTLS handshake attempt (FR-E09). [FR-072, FR-E09]
- [x] T056 Implement `nix-key status`: query daemon for: running state, socket path, connected devices count, total available keys, cert expiry warnings (within 30 days). [FR-073, FR-032]
- [x] T057 Implement `nix-key export <key-id>`: query daemon for key by SHA256 fingerprint (or unique prefix), print SSH public key format to stdout. Error if key not found or ambiguous prefix. [FR-074]
- [x] T058 Implement `nix-key config`: read and pretty-print `~/.config/nix-key/config.json`. Mask sensitive paths (show "present" not full path). [FR-075]
- [x] T059 Implement `nix-key logs`: tail systemd journal for user unit `nix-key-agent`. Parse JSON log entries, format human-readable with colors (level-colored prefix, timestamp, message, key fields). [FR-076]
- [ ] T060 Implement `nix-key test <device>`: resolve device from registry, mTLS dial to phone, call Ping RPC, report success with round-trip latency. On failure: report specific error (unreachable, cert mismatch, timeout). [FR-077]
- [ ] T061 Write CLI integration tests: start daemon with test fixtures, run each subcommand, verify output format and state changes. Test error cases: revoke nonexistent device, export unknown key, test unreachable device. [Story 5, SC-005]

## Phase 13: Android Emulator E2E Harness + Tests

### 13a: APK Build Infrastructure
- [ ] T062 Create Nix expression for Android APK build: either a `nix/android-apk.nix` derivation using `androidenv` from nixpkgs (preferred for reproducibility) or a documented Gradle build step with pinned SDK/NDK versions. The APK must include the gomobile AAR from `pkg/phoneserver`. Verify APK installs on emulator via `adb install`. [Build infra]

### 13b: Android Emulator Nix Infrastructure
- [ ] T063 Create `nix/android-emulator.nix`: Nix expression for Android emulator setup using `androidenv.emulateApp` or `androidenv.androidPkgs`. Configure: system image (API 34, x86_64), AVD with 2GB RAM, swiftshader GPU for headless rendering (no host GPU required), KVM acceleration. Create helper script `start-emulator.sh` that boots emulator, waits for `adb shell getprop sys.boot_completed` (with 120s timeout + retry), and returns. Test: emulator boots in Nix sandbox. [E2E infra]

### 13c: Test-Mode Deep Link for QR Bypass
- [ ] T064 Add test-mode intent handler to Android app: `nix-key://pair?payload=<base64-json>` deep link that bypasses ML Kit camera scanner and directly processes the QR payload. Only enabled when app is built with `BUILD_TYPE=debug` or a test flag. This is critical for emulator E2E — camera injection is unreliable. Write instrumented test: send intent via `adb am start`, verify pairing screen shows correct host info. [E2E infra, FR-022]

### 13d: UI Automator Test Helper Library
- [ ] T065 Create `android/app/src/androidTest/java/.../e2e/NixKeyE2EHelper.kt`: reusable UI Automator helper with methods:
  - `waitForApp(timeout)` — wait for MainActivity to be visible
  - `navigateToKeys()` — navigate to key management screen
  - `createKey(name, type)` — tap FAB, fill form, submit, wait for key to appear in list
  - `pairWithHost(qrPayload)` — send deep link intent, wait for confirmation dialog, tap Accept
  - `approveSignRequest(timeout)` — wait for SignRequestDialog to appear, tap Approve
  - `denySignRequest()` — wait for dialog, tap Deny
  - `enterTailscaleAuthKey(key)` — on auth screen, enter key, tap connect, wait for success
  - `waitForElement(selector, timeout)` — generic wait with configurable timeout
  Each method includes retry logic (3 attempts) for UI flakiness. Write a self-test that exercises each helper against the app on a local emulator. [E2E infra]

### 13e: Android Emulator E2E Test
- [ ] T066 Write Android emulator E2E test as `test/e2e/android_e2e_test.sh` (shell orchestrator). **Architecture: side-by-side, NOT nested.** GitHub Actions runners support KVM but NOT nested KVM (the runner is already a VM). The Android emulator and NixOS VM must run side-by-side on the same host, both using KVM directly — never an emulator inside a NixOS VM. The test orchestrator coordinates both via `adb` (emulator) and `ssh`/CLI (NixOS VM or native host processes).

  Test layout on CI runner:
  ```
  CI Runner (KVM available)
  ├── headscale (native process or container)
  ├── tailscaled (host node, joined to headscale)
  ├── nix-key daemon (native process, using host tailscale)
  ├── Android Emulator (QEMU+KVM, direct on runner)
  └── test/e2e/android_e2e_test.sh (orchestrator)
  ```

  For local dev: same layout, everything runs on the dev machine natively.

  Steps:
  1. Start headscale → create namespace + 2 pre-auth keys
  2. Start host tailscaled (join headscale) + nix-key daemon
  3. Boot Android emulator (from T063), install APK (from T062)
  4. Inject Tailscale auth key via `NixKeyE2EHelper.enterTailscaleAuthKey()`
  5. Create Ed25519 key via `NixKeyE2EHelper.createKey("test-key", "ed25519")`
  6. Run `nix-key pair` on host (auto-confirm) → capture QR payload
  7. Pair phone via `NixKeyE2EHelper.pairWithHost(qrPayload)` (deep link, T064)
  8. Verify device appears in `nix-key devices`
  9. Trigger SSH sign request on host via `ssh-add -L` + `ssh-keygen -Y sign`
  10. `NixKeyE2EHelper.approveSignRequest(30s)` on emulator
  11. Verify SSH operation succeeds
  12. Test denial: trigger sign, `NixKeyE2EHelper.denySignRequest()`, verify SSH_AGENT_FAILURE
  Timeout budget: 5 minutes total. Retry wrapper: 2 attempts (emulator flakiness). [T-AI-*, Story 1, Story 2, Story 3]

## Phase 14: CI/CD Pipeline + Release

### 14a: CI Workflow with Structured Output
- [ ] T067 Create `.github/workflows/ci.yml`: on PR to develop. Parallel jobs: lint (golangci-lint + ktlint + nixfmt), test-host (`nix flake check` — Go tests + NixOS VM tests), test-android (Gradle build + instrumented tests). Security job (on every push to main AND on PRs): Tier 1 (Trivy, Semgrep, Gitleaks, govulncheck) + Tier 1.5 (Snyk, SonarCloud, OpenSSF Scorecard — for public repo). All must pass for merge. Use Cachix for Nix binary cache (configure cache name + CACHIX_AUTH_TOKEN secret). **Each job MUST**: (a) upload `test-logs/` directory as workflow artifact on failure via `actions/upload-artifact`, (b) output a structured job summary to `$GITHUB_STEP_SUMMARY` with pass/fail counts and failure names. [CI/CD]

### 14b: CI Failure Summary Artifact
- [ ] T068 Create `scripts/ci-summary.sh`: script that runs after all test jobs, collects `test-logs/summary.json` from each job, and produces `ci-summary.json` with: `{jobs: [{name, pass, fail, skip, duration, failures}], overall: "pass"|"fail", artifactUrls: {}}`. Upload as workflow artifact. This is the structured entry point for fix-validate agents to diagnose CI failures without parsing raw logs. [CI/CD debugging]

### 14c: E2E Workflow with Retry
- [ ] T069 Create `.github/workflows/e2e.yml`: on push to develop (after CI passes). Android emulator E2E on KVM-enabled runner (`ubuntu-latest` with KVM). Retry wrapper: 3 attempts with 60s cooldown between attempts (emulator flakiness). Upload `test-logs/` + emulator logcat as artifacts on failure. Timeout: 15 minutes per attempt. [CI/CD]

### 14d: Release Pipeline
- [ ] T070 Create `.github/workflows/release.yml`: on push to main. Full CI + security + E2E. Build: `nix build` (Go binary for x86_64-linux + aarch64-linux), Gradle `assembleRelease` (APK). SBOM: Trivy CycloneDX. Version: use `release-please` with conventional commits for automated semantic versioning (configure `.release-please-manifest.json` + `release-please-config.json`). Create GitHub Release with binary + APK + SBOM attached. [CI/CD]

### 14e: Branch Protection + Verification
- [ ] T071 Set up branch protection: main requires all CI + security + E2E green. develop requires lint + test-host + test-android green. Configure `release-please` to auto-create release PRs on develop→main merges. [CI/CD]
- [ ] T072 Verify release pipeline end-to-end: push to develop → CI green → merge to main → `release-please` creates release PR → merge release PR → GitHub Release created with binaries + APK + SBOM. Verify artifacts downloadable and binary runs `nix-key --help`. [CI/CD, SC-009]

## Post-Implementation

- [ ] T073 REVIEW: Code review of all implementation. Security focus: mTLS correctness, cert pinning, age encryption, no plaintext secrets, input validation on all gRPC messages, no secret material in CI logs/artifacts. Write REVIEW-TODO.md with findings. Fix all critical/high findings. [Code review]
- [ ] T074 Local smoke test: build Go binary via `nix build`, install APK on emulator. Walk through: service starts → pair phone → create key → ssh-add -L → SSH sign succeeds → revoke device. Cold-start test: delete all state, verify first-run works. Warm-start test: verify second run is faster. [Smoke test]
- [ ] T075 CI/CD validation: push to develop, monitor CI with `gh run list`. On failure: download `ci-summary.json` artifact, diagnose, fix, push. Iterate (cap: 15 attempts). Once green: create PR to main. Verify `release-please` creates release PR. Merge. Verify GitHub Release with artifacts. [CI validation]
- [ ] T076 Update `CLAUDE.md` with final project structure, all available commands, test instructions, architecture overview, CI/CD debugging instructions (how to read `ci-summary.json`, where to find test-logs artifacts). Update `UI_FLOW.md` to reflect final implementation. [Documentation]

---

## Phase Dependencies Summary

```
T001-T006 (Phase 1) → T007-T011 (Phase 2) → T012-T014 (Phase 3) → T015-T018 (Phase 4)
                                                                   → T019-T022 (Phase 5)
                                                                   → T023-T027 (Phase 6)
T001 → T028-T032 (Phase 7) [parallel with Phases 3-6]
T028-T032 → T033-T038 (Phase 8) [parallel with Phases 5-6]
T019-T027 + T039-T044 → T045-T048 (Phase 10)
T033-T038 + T039-T044 → T045-T048 (Phase 10)
T045-T048 → T049-T053 (Phase 11) [parallel with Phase 12]
T023-T027 + T039-T044 → T054-T061 (Phase 12) [needs daemon + control socket + NixOS module, NOT Phase 10 E2E]
T033 → T062 (APK build) [can start after gomobile bridge]
T054-T061 → T063-T066 (Phase 13: emulator harness + E2E)
T062-T066 → T067-T072 (Phase 14: CI/CD)
T072 → T073-T076 (Post-Implementation)
```

### Phase 13 Internal Dependencies

```
T062 (APK build) ─────────────────────────────┐
T063 (emulator Nix) ──────────────────────────┤
T064 (deep link QR bypass) ───────────────────┤→ T066 (E2E test)
T065 (UI Automator helpers) ──────────────────┘
```

T062-T065 can all be built in parallel. T066 requires all four.

### Phase 14 Internal Dependencies

```
T067 (CI workflow) → T068 (CI summary) → T072 (verify pipeline)
T069 (E2E workflow, depends on T066) ──────┘
T070 (release workflow) ───────────────────┘
T071 (branch protection) ─────────────────┘
```

### Parallel Agent Strategy

```
Agent A (Host):    T001→T006 → T007→T011 → T012→T014 → T015→T018 → T019→T022 → T023→T027 → T039→T044 → T045→T048 → T049→T053 → T054→T061 → T067→T072
Agent B (Android): (wait for T006) → T028→T032 → T033→T038 → (wait for T044) → T045 collab → T062→T066 → (wait for T067) → T072 collab
Sync points: T044 (NixOS module ready), T048 (E2E phonesim ready), T066 (Android E2E ready), T072 (CI verified)
```
