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
- [x] T053 Write distributed trace E2E test: NixOS VM with OTEL collector (or Jaeger) + headscale + daemon + phonesim (with OTEL). Perform sign request. Query collector API. Verify: trace exists, host spans present, phone spans present, phone spans are children of host spans via traceparent. [T-E2E-03, Story 7, SC-008]

## Phase 12: CLI Polish

- [x] T054 Implement `nix-key devices`: query daemon via control socket, format as table (name, Tailscale IP, cert fingerprint, last seen, status). [FR-071]
- [x] T055 Implement `nix-key revoke <device>`: send revoke to daemon, daemon removes device from registry, deletes cert files. Confirm deletion on stdout. Reject if device is Nix-declared (print error directing user to remove from Nix config). Test that after revocation, the revoked device's cert is rejected on mTLS handshake attempt (FR-E09). [FR-072, FR-E09]
- [x] T056 Implement `nix-key status`: query daemon for: running state, socket path, connected devices count, total available keys, cert expiry warnings (within 30 days). [FR-073, FR-032]
- [x] T057 Implement `nix-key export <key-id>`: query daemon for key by SHA256 fingerprint (or unique prefix), print SSH public key format to stdout. Error if key not found or ambiguous prefix. [FR-074]
- [x] T058 Implement `nix-key config`: read and pretty-print `~/.config/nix-key/config.json`. Mask sensitive paths (show "present" not full path). [FR-075]
- [x] T059 Implement `nix-key logs`: tail systemd journal for user unit `nix-key-agent`. Parse JSON log entries, format human-readable with colors (level-colored prefix, timestamp, message, key fields). [FR-076]
- [x] T060 Implement `nix-key test <device>`: resolve device from registry, mTLS dial to phone, call Ping RPC, report success with round-trip latency. On failure: report specific error (unreachable, cert mismatch, timeout). [FR-077]
- [x] T061 Write CLI integration tests: start daemon with test fixtures, run each subcommand, verify output format and state changes. Test error cases: revoke nonexistent device, export unknown key, test unreachable device. [Story 5, SC-005]

## Phase 13: Android Emulator E2E Harness + Tests

### 13a: APK Build Infrastructure
- [x] T062 Create Nix expression for Android APK build: either a `nix/android-apk.nix` derivation using `androidenv` from nixpkgs (preferred for reproducibility) or a documented Gradle build step with pinned SDK/NDK versions. The APK must include the gomobile AAR from `pkg/phoneserver`. Verify APK installs on emulator via `adb install`. [Build infra]

### 13b: Android Emulator Nix Infrastructure
- [x] T063 Create `nix/android-emulator.nix`: Nix expression for Android emulator setup using `androidenv.emulateApp` or `androidenv.androidPkgs`. Configure: system image (API 34, x86_64), AVD with 2GB RAM, swiftshader GPU for headless rendering (no host GPU required), KVM acceleration. Create helper script `start-emulator.sh` that boots emulator, waits for `adb shell getprop sys.boot_completed` (with 120s timeout + retry), and returns. Test: emulator boots in Nix sandbox. [E2E infra]

### 13c: Test-Mode Deep Link for QR Bypass
- [x] T064 Add test-mode intent handler to Android app: `nix-key://pair?payload=<base64-json>` deep link that bypasses ML Kit camera scanner and directly processes the QR payload. Only enabled when app is built with `BUILD_TYPE=debug` or a test flag. This is critical for emulator E2E — camera injection is unreliable. Write instrumented test: send intent via `adb am start`, verify pairing screen shows correct host info. [E2E infra, FR-022]

### 13d: UI Automator Test Helper Library
- [x] T065 Create `android/app/src/androidTest/java/.../e2e/NixKeyE2EHelper.kt`: reusable UI Automator helper with methods:
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
- [x] T066 Write Android emulator E2E test as `test/e2e/android_e2e_test.sh` (shell orchestrator). **Architecture: side-by-side, NOT nested.** GitHub Actions runners support KVM but NOT nested KVM (the runner is already a VM). The Android emulator and NixOS VM must run side-by-side on the same host, both using KVM directly — never an emulator inside a NixOS VM. The test orchestrator coordinates both via `adb` (emulator) and `ssh`/CLI (NixOS VM or native host processes).

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
- [x] T067 Create `.github/workflows/ci.yml`: on PR to develop. Parallel jobs: lint (golangci-lint + ktlint + nixfmt), test-host (`nix flake check` — Go tests + NixOS VM tests), test-android (Gradle build + instrumented tests). Security job (on every push to main AND on PRs): Tier 1 (Trivy, Semgrep, Gitleaks, govulncheck) + Tier 1.5 (Snyk, SonarCloud, OpenSSF Scorecard — for public repo). All must pass for merge. Use Cachix for Nix binary cache (configure cache name + CACHIX_AUTH_TOKEN secret). **Each job MUST**: (a) upload `test-logs/` directory as workflow artifact on failure via `actions/upload-artifact`, (b) output a structured job summary to `$GITHUB_STEP_SUMMARY` with pass/fail counts and failure names. [CI/CD]

### 14b: CI Failure Summary Artifact
- [x] T068 Create `scripts/ci-summary.sh`: script that runs after all test jobs, collects `test-logs/summary.json` from each job, and produces `ci-summary.json` with: `{jobs: [{name, pass, fail, skip, duration, failures}], overall: "pass"|"fail", artifactUrls: {}}`. Upload as workflow artifact. This is the structured entry point for fix-validate agents to diagnose CI failures without parsing raw logs. [CI/CD debugging]

### 14c: E2E Workflow with Retry
- [x] T069 Create `.github/workflows/e2e.yml`: on push to develop (after CI passes). Android emulator E2E on KVM-enabled runner (`ubuntu-latest` with KVM). Retry wrapper: 3 attempts with 60s cooldown between attempts (emulator flakiness). Upload `test-logs/` + emulator logcat as artifacts on failure. Timeout: 15 minutes per attempt. [CI/CD]

### 14d: Release Pipeline
- [x] T070 Create `.github/workflows/release.yml`: on push to main. Full CI + security + E2E. Build: `nix build` (Go binary for x86_64-linux + aarch64-linux), Gradle `assembleRelease` (APK). SBOM: Trivy CycloneDX. Version: use `release-please` with conventional commits for automated semantic versioning (configure `.release-please-manifest.json` + `release-please-config.json`). Create GitHub Release with binary + APK + SBOM attached. [CI/CD]

### 14e: Branch Protection + Verification
- [x] T071 Set up branch protection: main requires all CI + security + E2E green. develop requires lint + test-host + test-android green. Configure `release-please` to auto-create release PRs on develop→main merges. [CI/CD]
- [x] T072 Verify release pipeline end-to-end: push to develop → CI green → merge to main → `release-please` creates release PR → merge release PR → GitHub Release created with binaries + APK + SBOM. Verify artifacts downloadable and binary runs `nix-key --help`. [CI/CD, SC-009]

## Post-Implementation

- [x] T073 REVIEW: Code review of all implementation. Security focus: mTLS correctness, cert pinning, age encryption, no plaintext secrets, input validation on all gRPC messages, no secret material in CI logs/artifacts. Write REVIEW-TODO.md with findings. Fix all critical/high findings. [Code review]
- [x] T074 Local smoke test: build Go binary via `nix build`, install APK on emulator. Walk through: service starts → pair phone → create key → ssh-add -L → SSH sign succeeds → revoke device. Cold-start test: delete all state, verify first-run works. Warm-start test: verify second run is faster. [Smoke test]
- [x] T075 [needs: gh, ci-loop] CI/CD validation: push to develop, iterate until CI green, create PR to main. Verify `release-please` creates release PR. [CI validation]
- [x] T076 Update `CLAUDE.md` with final project structure, all available commands, test instructions, architecture overview, CI/CD debugging instructions (how to read `ci-summary.json`, where to find test-logs artifacts). Update `UI_FLOW.md` to reflect final implementation. [Documentation]

## Phase 15: Host Hardening

### 15a: Fuzz Testing [P with 15b, 15c, 15d]

- [x] T077 Add Go fuzz targets (`testing.F`) for all untrusted-input boundaries: (1) SSH agent protocol message parsing (`internal/agent/`), (2) protobuf message deserialization, (3) QR code payload JSON parsing (`internal/pairing/qr.go`), (4) config JSON parsing (`internal/config/`), (5) certificate PEM parsing (`internal/mtls/certs.go`), (6) control socket JSON protocol parsing (`internal/daemon/control.go`). Seed corpus from existing test fixtures in `testdata/fuzz/`. Property tests: `decode(encode(x)) == x` for protobuf round-trips. Commit `testdata/fuzz/` as regression corpus. [T-FZ-01 through T-FZ-06, SC-012]
  **Done**: 6+ fuzz targets exist, seed corpus committed, `go test` runs seed corpus as regression.

- [x] T078 Add fuzz CI integration. In `ci.yml`: seed corpus runs on every PR (already happens via `go test`). Add time-boxed generative fuzzing (60s per target) to PR and develop-push workflows. Add `.github/workflows/fuzz.yml` for scheduled nightly deep fuzzing. [SC-012, CI/CD]
  **Done**: Fuzz runs on PRs/develop, nightly workflow exists, crashes auto-saved to `testdata/fuzz/`.

### 15b: Performance & Latency Testing [P with 15a, 15c, 15d]

- [x] T079 Add E2E latency assertion test: start agent + in-process gRPC phone server with auto-approve, run 20 sign requests, assert p95 < 2s (skip with `-short`). Add microbenchmarks: `BenchmarkMTLSHandshake` (`internal/mtls/`), `BenchmarkGRPCRoundTrip` (`pkg/phoneserver/`), `BenchmarkAgeDecrypt` (`internal/mtls/age.go`). Add `make bench` target to Makefile. CI: > 300% regression fails build. [T-PF-01 through T-PF-04, SC-013]
  **Done**: Latency test exists with p95 check, 3 microbenchmarks exist, `make bench` works.

### 15c: Adversarial VM Tests [P with 15a, 15b, 15d]

- [x] T080 Add adversarial cert fixtures to `test/fixtures/gen/`: deterministic adversarial certificates — (1) expired client cert, (2) not-yet-valid cert, (3) cert signed by different CA, (4) cert with wrong EKU, (5) valid cert not in trust store (unpaired device). All from fixed seeds. Add to `make generate-fixtures`. [T-ADV-01 through T-ADV-06] [produces: adversarial cert fixtures]
  **Done**: `test/fixtures/adversarial/` contains all 5 cert types.

- [x] T081 Write `nix/tests/adversarial-test.nix`: NixOS VM test with `rogue` node alongside legitimate host and phonesim. Tests: (1) expired cert → rejected, (2) wrong-CA cert → rejected, (3) unpaired cert → rejected, (4) connection on non-Tailscale interface (raw eth0) → rejected, (5) replayed pairing token → rejected, (6) error responses leak no internal details. Use `host.fail(...)` for adversarial assertions. Enable firewall. [T-ADV-01 through T-ADV-06, SC-011] [consumes: adversarial cert fixtures]
  **Done**: VM test passes all 6 scenarios with firewall enabled.

### 15d: Local Security Scan + DX [P with 15a, 15b, 15c]

- [x] T082 Add `make security-scan` Makefile target: runs Trivy, Semgrep (with `p/golang` config), Gitleaks, govulncheck. JSON output to `test-logs/security/<scanner>.json`. Aggregate into `test-logs/security/summary.json` with `{scanners: {name: {findings: N, exit_code: N}}, total_findings: N, pass: bool}`. Add `make validate` target: `make test && make lint && make security-scan`. [T-DX-07, T-DX-08, SC-022]
  **Done**: Both targets work, summary.json generated, validate exits 0 only when all pass.

- [x] T083 Update CI security job to produce JSON alongside SARIF. For each scanner, add parallel JSON output to `test-logs/security/`. Upload `test-logs/security/` as workflow artifact on every run. Update `scripts/ci-summary.sh` to include per-scanner finding counts in security job entry. [CI/CD, SC-009]
  **Done**: CI produces SARIF + JSON, artifacts uploaded, ci-summary.json has per-scanner details.

### 15e: DX Validation + Cold-Start [depends on 15d]

- [x] T084 Verify all Makefile targets work: `make test`, `make test-unit`, `make test-integration`, `make lint`, `make build`, `make proto`, `make bench`, `make security-scan`, `make validate`, `make cover`, `make generate-fixtures`, `make clean`, `make clean-all`. Each must exit 0 and produce expected output. [T-DX-01 through T-DX-11, SC-022]
  **Done**: All targets verified.

- [x] T085 Add automated cold-start and idempotency tests. Cold-start: delete all state dirs → start daemon → verify dirs created with 0700 permissions, daemon starts. Warm-start: stop/restart → reuses state, no errors. Pairing idempotency: `nix-key pair` setup twice → no corruption, age identity skip-if-exists. Secrets at rest: verify private keys on disk are age-encrypted after pairing, daemon decrypts into memory only, `ageKeyFile` option works. [T-CS-01 through T-CS-03, T-HI-19, SC-020, SC-021]
  **Done**: Cold-start, warm-start, idempotency, and secrets-at-rest tests pass.

- [x] T086 Add missing host integration tests: cert expiry warning in `nix-key status` (T-HI-11), two cert pairs generated during pairing (T-HI-12), age decrypt failure at startup (T-HI-13), deleted key sign request (T-HI-14), atomic pairing on write failure (T-HI-15), concurrent pairing rejection (T-HI-16), error hierarchy validation (T-HI-17), logging validation (T-HI-18), control socket test (T-HI-20), pairing without Tailscale (T-HI-21), tracing disabled = no overhead (T-HI-09a). [SC-018, SC-019, SC-020]
  **Done**: All new T-HI tests pass.

## Phase 16: Android Hardening

- [x] T087 Implement key unlock lifecycle (FR-116 through FR-119). Two independent per-key policies: unlock policy (none/biometric/password/biometric+password, default: password) and signing policy (always-ask/biometric/password/biometric+password/auto-approve, default: biometric). Sign request on locked key auto-triggers unlock prompt showing key name + host name. Eager decrypt on app start for none-unlock keys. Key material persists in memory across background/foreground, wiped on process kill. Manual re-lock from key detail or long-press. Queued requests retry own unlock if prior unlock fails (FR-053). Re-lock while queued triggers fresh unlock (FR-117). Security warning for none-unlock and auto-approve signing (FR-046). [FR-045, FR-046, FR-051, FR-116-119, FR-E13, T-AI-04, T-AI-11, T-AI-12]
  **Done**: Both policies configurable per key, unlock lifecycle works, all edge cases covered.

- [x] T088 Implement persistent Tailnet connection indicator across all screens (FR-110): green/Connected, yellow/Connecting, red/Disconnected. Implement per-key lock/unlock indicator on key list reflecting runtime decrypt state (FR-111). [T-UI-01, T-UI-02, SC-014]
  **Done**: Indicators visible on all screens, correct states.

- [x] T089 Implement loading states for all async operations. Tailscale auth: "Connecting to Tailnet..." with spinner, error+retry on failure/timeout (FR-112). Pairing: "Scanning...", "Connecting to host...", "Waiting for host approval..." with error+retry (FR-113). gRPC startup: "Starting nix-key..." notification, "nix-key active" only when listening (FR-114). Stale auth: re-auth flow instead of crash (FR-115). Port conflict: specific error notification (FR-E19). [T-UI-03, T-UI-04, SC-014]
  **Done**: All async ops show loading→ready or loading→error states, never premature "ready".

- [x] T090 Add `@GuardedBy`/`@ThreadSafe` annotations to all concurrent Kotlin code: GoPhoneServer, GrpcServerService, KeyManager, HostRepository, TailscaleManager. Add Infer/RacerD to Nix devshell. Run `infer run --racerd-only -- ./gradlew assembleDebug`. Add to CI lint job. Fix any races found. [Concurrency, Security]
  **Done**: All concurrent code annotated, RacerD runs clean.

- [x] T091 Add ML Kit `InputImage.fromBitmap()` instrumented test: generate QR code bitmap with known payload, feed to ML Kit barcode scanner, verify correct payload extraction. Tests the full decode→parse path without camera. [T-QR-01, SC-015]
  **Done**: QR bitmap test passes.

- [x] T092 Add multi-host pairing test: pair phone with two mock hosts, verify both stored in EncryptedSharedPreferences, sign requests from each host work independently. [T-AI-18, FR-030]
  **Done**: Multi-host pairing works.

- [x] T093 Add remaining Android tests: security warnings for auto-approve/none-unlock (T-AI-13), display name editing (T-AI-14), Android structured logging with trace correlation (T-AI-15), expired cert mid-session behavior (T-AI-16), gRPC port conflict error (T-AI-17). [FR-046, FR-048, FR-093, FR-094, FR-E18, FR-E19]
  **Done**: All new T-AI tests pass.

- [x] T094 Update `data-model.md`: replace "confirmation policy" with unlock policy + signing policy split. Update SSHKey entity state transitions to include locked/unlocked runtime state. Update `UI_FLOW.md`: replace all "confirmation policy" references with two-policy model, add Tailnet indicator and per-key lock indicator to screen descriptions, add loading states to relevant screens. [Stale terminology fix]
  **Done**: data-model.md and UI_FLOW.md consistent with spec two-policy model.

## Phase 17: Documentation & License

- [x] T095 Create `README.md` at repository root. Sections: (1) Title + tagline, (2) Badges (CI, coverage, license, release), (3) Description (what/why/differentiator), (4) Architecture diagram (Mermaid: host↔Tailscale↔phone), (5) Features list, (6) Getting Started (prerequisites, install via flake + channel, first run), (7) Configuration table (all `services.nix-key` options with Key/Type/Default/Required/Sensitive/Description), (8) Usage (CLI subcommands with examples), (9) Development (nix develop, make test, project structure), (10) CI Setup (required secrets: SNYK_TOKEN, SONAR_TOKEN, CACHIX_AUTH_TOKEN with how-to-obtain), (11) Security (threat model, cert pinning, non-extractable keys), (12) License. [SC-016]
  **Done**: README renders, all commands work, badges live.

- [x] T096 Create `specs/nix-key/coverage-boundaries.md`. Document: fully tested in CI (every PR): host Go with -race, NixOS VM tests. On develop push: Android emulator E2E, fuzz generative. Not in CI: real Keystore hardware, real biometrics, real Tailscale auth, real camera QR. Mitigations: interfaces tested via fakes, protocol via phonesim, camera via ML Kit bitmap test. Per-component thresholds. [Documentation]
  **Done**: Coverage boundaries documented.

- [x] T097 Determine license. Check all dependency licenses (`go-licenses`, Gradle license plugin). If all compatible with MIT, use MIT. Otherwise use least restrictive compatible license. Create LICENSE file. [SC-017]
  **Done**: LICENSE file present, compatible with all deps.

- [x] T098 Update `CLAUDE.md` (was T076): final project structure, all commands including new `make bench`, `make security-scan`, `make validate`, architecture overview, CI/CD debugging, two-policy model summary, new phases. [Documentation]
  **Done**: CLAUDE.md reflects final state.

## Phase 18: CI Hardening & Non-Vacuous Validation [Story 8]

- [x] T100 [P] Fix exit code swallowing in test-android Gradle step: add `set -o pipefail` before `./gradlew assembleDebug testDebugUnitTest` in `.github/workflows/ci.yml` [FR-200]
  **Done when**: Gradle failures cause the step to exit non-zero even when piped through tee.

- [x] T101 [P] Add "Verify tests ran" step to test-android job in `.github/workflows/ci.yml`: count JUnit XML files in `android/app/build/test-results/`, extract total test count, exit non-zero if 0 files or 0 tests, use `if: always()` with `::error::` annotation [FR-201, FR-206]
  **Done when**: test-android job fails with clear error when no tests ran.

- [x] T102 [P] Add "Verify tests ran" step to test-host job in `.github/workflows/ci.yml`: check `test-logs/ci/latest/summary.json` exists, parse `passed + failed` with jq, exit non-zero if total is 0 or file missing, use `if: always()` with `::error::` annotation [FR-202, FR-206]
  **Done when**: test-host job fails with clear error when no tests ran.

- [x] T103 [P] Add "Upload debug APK" step to test-android job in `.github/workflows/ci.yml`: after Gradle assembleDebug succeeds, use `actions/upload-artifact@v4` with name `debug-apk`, path `android/app/build/outputs/apk/debug/app-debug.apk` [FR-203]
  **Done when**: debug-apk artifact appears in GitHub Actions artifacts on develop builds.

- [x] T104 [P] Add "Upload Go binary" step to test-host job in `.github/workflows/ci.yml`: build with `nix build`, copy `result/bin/nix-key`, use `actions/upload-artifact@v4` with name `nix-key-binary` [FR-204]
  **Done when**: nix-key-binary artifact appears in GitHub Actions artifacts on develop builds.

- [x] T105 Add "Verify scanners ran" step to security job in `.github/workflows/ci.yml`: for each scanner (trivy, semgrep, gitleaks, govulncheck), check JSON output file exists and is >10 bytes, log `::warning::` for missing scanners (advisory, not hard failure), use `if: always()` [FR-205, FR-206]
  **Done when**: Security job logs show verification output for each scanner.

## Phase 19: Local CI Verification

- [x] T109a [P] Verify Go CI steps locally (fix-validate loop): run `nix build` and `go test -json -race -count=1 ./...`. Verify `result/bin/nix-key` exists and `test-logs/ci/latest/summary.json` exists with `passed + failed > 0`. On failure: fix and retry. [FR-202, FR-204]
  **Done when**: Go binary builds, Go tests pass with non-zero count, artifact paths verified. Fix-validate loop, 20-iteration cap.

- [x] T109b [P] Verify Android CI steps locally (fix-validate loop): run `./gradlew assembleDebug testDebugUnitTest --no-daemon` in `android/`. Verify `android/app/build/outputs/apk/debug/app-debug.apk` exists. Verify JUnit XML files exist in `android/app/build/test-results/` with >0 tests. On failure: fix (missing SDK, Gradle config, gomobile AAR) and retry. [FR-200, FR-201, FR-203]
  **Done when**: Android APK builds, Android tests pass with non-zero count, artifact paths verified. Fix-validate loop, 20-iteration cap.

- [x] T109c [P] Verify security scanner CI steps locally (fix-validate loop): run each scanner command from the security job (`trivy fs`, `semgrep scan`, `gitleaks detect`, `govulncheck`). Verify each produces JSON output >10 bytes in `test-logs/security/`. On failure: fix (missing scanner binary, wrong config) and retry. [FR-205]
  **Done when**: All 4 scanners produce non-empty JSON output. Fix-validate loop, 20-iteration cap.

## Phase 20: Android Build & Emulator Integration Fix

- [x] T110 Fix gomobile AAR build: the Nix-packaged gomobile (Dec 2024) is broken with Go 1.26.1 (`GOPATH=gomobile-work` relative path rejected). Either update gomobile in nixpkgs, patch the Nix derivation, or apply the CI workaround (`GOPATH`/`GOMODCACHE` override) to `nix/android-apk.nix`'s `build-android-apk` script and the Makefile `gomobile` target. Remove the stub AAR — the real gomobile AAR must build. [Build infra]
  **Done when**: `nix develop --command build-android-apk` succeeds end-to-end. `jar tf android/app/libs/phoneserver.aar` shows `.so` native libraries (not just Java stub classes). `make android-apk` also works.

- [ ] T111 [P] Verify gomobile AAR contains real Go code: run `jar tf android/app/libs/phoneserver.aar` and verify it contains `jni/*/libgojni.so` (ARM64, x86_64). Verify the AAR size is >1MB (stub AARs are <100KB). If the AAR is a stub, T110 is not done. [Build infra]
  **Done when**: AAR contains native .so files, size >1MB.

- [ ] T112 Run Android instrumented tests on local emulator: boot emulator via `nix develop --command start-emulator` (or `nix/android-emulator.nix` helper). Install debug APK via `adb install`. Run `./gradlew connectedDebugAndroidTest`. Parse JUnit XML results. Fix any failures — especially tests that call Go code via the bridge (GoPhoneServer, PhoneServer), which will crash if the AAR is a stub. [Android E2E]
  **Done when**: Emulator boots, APK installs, `connectedDebugAndroidTest` passes with >0 tests, JUnit XML in `app/build/outputs/androidTest-results/` shows real results.

- [ ] T113 Run E2E test script on local emulator: execute `test/e2e/android_e2e_test.sh` locally end-to-end. This orchestrates: emulator boot, APK install, nix-key daemon start, pairing via deep link, key creation, SSH sign flow. Fix any failures in the fix-validate loop. [Android E2E]
  **Done when**: `test/e2e/android_e2e_test.sh` passes locally with all steps completing (pair, create key, sign).

## Phase 21: Final CI Validation & PR

- [ ] T099 [needs: gh, ci-loop] Local validation first, then push to develop and iterate until CI green. Use fix-validate subagents for each local step. Steps: (1) Spawn parallel fix-validate subagents: (a) `make validate` (test + lint + security-scan), (b) `nix flake check` (Go tests + NixOS VM tests), (c) `make android-apk` (Android debug build). Each subagent loops until its command passes. (2) Only after all three pass, push to develop. (3) Iterate on CI-only failures (fuzz, artifacts, RacerD, emulator E2E) until fully green including non-vacuous validation steps and artifact uploads. (4) Create PR to main. [CI validation, FR-208]
  **Done when**: All local validations green (host tests, NixOS VM tests, Android build), CI fully green on develop with non-vacuous test counts, artifacts uploaded, PR to main created.

- [ ] T114 [needs: gh] Verify E2E workflow_run chain fires: push to develop, wait for CI to pass, then verify `e2e.yml` is triggered via `gh run list --workflow=e2e.yml`. The E2E workflow must reach `success` conclusion (not `skipped`). If the workflow_run trigger never fires, push a follow-up commit. Fix any E2E failures in CI. [CI validation]
  **Done when**: `gh run list --workflow=e2e.yml` shows at least one run with `success` conclusion triggered by a `workflow_run` event from CI.

- [ ] T106 [needs: gh] Observable output validation: after CI passes on develop, verify all expected artifacts are listed in the workflow run (`gh run view --json artifacts`), download debug-apk and nix-key-binary to confirm non-empty [SC-024]
  **Done when**: Both artifacts verified present and non-empty.

- [ ] T107 [needs: gh] Default branch readiness check: verify PR to main includes all workflow files, LICENSE, README, release config (`release-please-config.json`, `.release-please-manifest.json`). Verify `workflow_run` triggers in e2e.yml reference a workflow name that will exist on main after merge. [FR-208]
  **Done when**: PR diff confirms all required files reach main.

- [ ] T108 [needs: gh] Post-merge badge validation: after PR merges to main, fetch all 5 README badge URLs with curl, verify HTTP 200 and valid SVG content (no "not specified", "not found", 404). Document any badges that will self-heal after first release (GitHub release version badge). [FR-207, SC-025]
  **Done when**: CI, E2E, Release, License badges all render valid status. Release version badge documented as self-healing.

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
T023-T027 + T039-T044 → T054-T061 (Phase 12)
T033 → T062 (APK build)
T054-T061 → T063-T066 (Phase 13: emulator harness + E2E)
T062-T066 → T067-T072 (Phase 14: CI/CD)
T072 → T073-T075 (Post-Implementation: review, smoke, CI)
T075 → T077-T086 (Phase 15: Host Hardening) [15a-15d parallel, 15e depends on 15d]
T075 → T087-T094 (Phase 16: Android Hardening) [parallel with Phase 15]
T086 + T094 → T095-T098 (Phase 17: Documentation & License)
T098 → T100-T105 (Phase 18: CI Hardening) [all parallel]
T100-T105 → T109a, T109b, T109c (Phase 19: local CI verification) [all parallel]
T109a + T109b + T109c → T110-T111 (Phase 20: Android build & emulator fix) [parallel]
T110 → T112 (emulator instrumented tests)
T112 → T113 (E2E script locally)
T113 → T099 (Phase 21: final CI validation & PR)
T099 → T114 (verify workflow_run chain)
T099 → T106-T107 (Phase 21: Observable validation) [parallel]
T107 → T108 (Phase 21: Post-merge badge validation)
```

### Phase 15 Internal Dependencies

```
T077-T078 (fuzz) ──────────────────┐
T079 (bench) ──────────────────────┤
T080 → T081 (adversarial) ────────┤→ T084-T086 (DX validation + cold-start + missing tests)
T082-T083 (security scan + CI) ───┘
```

T077-T083 can all run in parallel (15a-15d). T084-T086 (15e) depend on T082 (needs `make validate`).

### Phase 16 Internal Dependencies

```
T087 (unlock lifecycle) ──────────┐
T088 (status indicators) ────────┤
T089 (loading states) ───────────┤→ T094 (update data-model.md + UI_FLOW.md)
T090 (RacerD) ───────────────────┤
T091 (QR bitmap test) ──────────┤
T092 (multi-host test) ──────────┤
T093 (remaining Android tests) ──┘
```

T087-T093 can all run in parallel. T094 depends on all (updates docs to reflect new code).

### Parallel Agent Strategy

```
Agent A (Host):    T001→...→T075 → T077-T086 (Phase 15) → T095-T098 (Phase 17) → T099
Agent B (Android): T001→...→T075 → T087-T094 (Phase 16) → (wait for T098) → T099
Agent C (Docs):    (wait for T086 + T094) → T095-T098 (Phase 17)
Sync points: T044, T048, T066, T072, T075 (hardening starts), T086+T094 (docs start), T098 (final CI)
```
