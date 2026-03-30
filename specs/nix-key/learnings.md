# Learnings

Discoveries, gotchas, and decisions recorded by the implementation agent across runs.

---

## T077 — Fuzz Testing

- `net.Pipe()` returns `net.Conn` which lacks `CloseWrite()`. Use `client.Close()` instead to signal EOF to the SSH agent server goroutine.
- Go fuzz seed corpus in `testdata/fuzz/` is automatically picked up alongside `f.Add()` seeds — both are run as regression tests during normal `go test`. No special flags needed.

## T078 — Fuzz CI Integration

- `go test -fuzz` only runs one fuzz target at a time (one `-fuzz` regex per invocation). To fuzz multiple targets, loop over each package/function pair with separate `go test` calls.
- Use `-run='^$'` alongside `-fuzz` to skip non-fuzz tests during generative fuzzing — otherwise all regular tests in the package also run each iteration.

## T079 — Performance & Latency Testing

- `BenchmarkMTLSHandshake` must create a new TCP listener per iteration (can't reuse TLS connections for handshake measurement). Using `net.Listen("tcp", "127.0.0.1:0")` inside the benchmark loop is the simplest correct approach.
- The E2E latency test (`TestIntegrationE2ESignLatency`) reuses `setupTestBackend` and `startTestAgent` helpers from `backend_test.go`, keeping it in `agent_test` package. The test populates the key cache via `client.List()` before timing sign requests to avoid measuring cache-miss overhead.

## T087 — Key Unlock Lifecycle

- The existing `ConfirmationPolicy` enum becomes the "signing policy" (per-sign behavior), while the new `UnlockPolicy` enum controls unlock-to-use behavior. Both are independent per-key settings stored separately in EncryptedSharedPreferences.
- `KeyUnlockManager` uses `ConcurrentHashMap` for thread-safe unlock state — no lock contention with the sign request queue. State is exposed as `StateFlow<Set<String>>` of fingerprints for reactive UI updates.
- `combinedClickable` (from `foundation`) requires `@OptIn(ExperimentalFoundationApi::class)` — don't import `clickable` alongside it or lint will flag unused imports.

## T080 — Adversarial Cert Fixtures

- The existing `generateCA` function hardcodes `CommonName: "nix-key Test CA"`. For the rogue CA fixture, a separate `generateRogueCA` function is needed with a distinct CN and serial number, otherwise tests asserting the rogue CA differs from the legitimate one will fail.

## T088 — Status Indicators

- `TailscaleManager` uses `AtomicReference`/`AtomicBoolean` (not `StateFlow`) for internal state, so UI can't reactively observe connection changes without adding an explicit `StateFlow`. Added `connectionState: StateFlow<TailnetConnectionState>` alongside the existing atomic fields.
- `CompositionLocalProvider` with a `compositionLocalOf<StateFlow<...>>` is the cleanest way to make a singleton's state available across all screens without threading parameters through the nav graph or injecting the manager into every ViewModel.
- `KeyDetailViewModel` loaded `isUnlocked` once at init and never updated it reactively. Collecting `keyUnlockManager.unlockedFingerprints` in `viewModelScope` fixes this so lock/unlock from other screens (e.g., long-press in key list) is reflected immediately.

## T089 — Loading States

- Go gRPC server port binding errors from gomobile may be wrapped in a generic `Exception` rather than `BindException`. Check both `e.cause is BindException` and message substring matching for "address already in use" / "EADDRINUSE" to reliably detect port conflicts.
- For connection timeout in ViewModels, use a child coroutine with `delay()` + state check rather than `withTimeout()`, because `withTimeout` would cancel the parent coroutine and prevent clean error state updates. The child timeout job is cancelled on success.

## T081 — Adversarial VM Test

- The rogue node running `openssl s_server` with adversarial certs needs firewall ports opened on the `tailscale0` interface. Without this, the host daemon cannot reach the TLS servers, causing connection timeout instead of cert-validation rejection.
- `grpc.NewClient` (gRPC-go v1.67+) is lazy — it does not dial on creation. TLS cert verification via `VerifyPeerCertificate` only runs when the first RPC (e.g., `Ping`) triggers the actual connection. So `mtls.DialMTLS` returns successfully; the cert failure appears as a Ping RPC error.
- In NixOS VM tests with 3 nodes (host, phone, rogue) all joining headscale, create a separate preauthkey for each node. Reusing the same key works but makes debugging harder when tailnet issues arise.

## T090 — Thread-safety annotations + RacerD

- GoPhoneServer has a real data race: `phoneServer` and `serverThread` are written after `running.getAndSet(true)` in `start()` but read after `running.getAndSet(false)` in `stop()`. The AtomicBoolean CAS establishes happens-before only for the atomic variable itself, not for subsequent non-volatile field writes. Fix: add `@Volatile` to both fields.
- `@ThreadSafe` requires `com.google.code.findbugs:jsr305:3.0.2` dependency. AndroidX has `@GuardedBy` but NOT `@ThreadSafe`. Add `jsr305` as an `implementation` dependency.
- Infer v1.2.0 pre-built Linux binary is ~500MB. For the nix package, it needs `autoPatchelfHook` plus runtime deps: `gmp`, `mpfr`, `sqlite`, `zlib`, `stdenv.cc.cc.lib`.

## T091 — ML Kit QR bitmap test

- Use `Base64.NO_WRAP` (not `Base64.DEFAULT`) when generating QR payloads for ZXing — newlines in the base64 string add unnecessary data to the QR code. Android's `Base64.decode(str, Base64.DEFAULT)` in `decodeQrPayload` accepts both wrapped and unwrapped input.
- ZXing `QRCodeWriter` is added as `androidTestImplementation` only (version 3.5.3). For larger payloads (multi-line PEM certs), increase bitmap size to 800px to ensure reliable ML Kit detection.

## T082 — Security scan Makefile targets

- govulncheck JSON output is newline-delimited JSON objects (not a JSON array). Use `jq -s '[.[] | select(.finding != null)] | length'` to count findings.
- gitleaks with 0 findings writes an empty JSON array `[]` to the report file, not `{}` or nothing. Use `jq 'if type == "array" then length else 0 end'` to count.

## T083 — CI security JSON output

- Trivy action doesn't support dual output formats in one invocation. Run it twice: once for SARIF (with `exit-code: "1"` for CI gating), once for JSON (with `exit-code: "0"` and `continue-on-error: true` so it always produces the file).
- The `security-logs` artifact path was changed from `test-logs/` (entire directory) to `test-logs/security/` (scoped). The `ci-summary.sh` `find_summary` function searches `<artifacts-dir>/security-logs/` so the summary.json is found at `security-logs/security/summary.json` (artifact name / subpath).

## T092 — Multi-host pairing test

- `GoPhoneServer` constructor requires `KeyUnlockManager` (added in T087). The existing `GoPhoneServerTest` still passes only 2 args (keyManager, signRequestQueue) — this is a latent compile error. New tests must pass all 3 args.
- When testing sign requests through `GoPhoneServer`, keys must be pre-unlocked via `keyUnlockManager.unlock(keyInfo)` to avoid `needsUnlock=true` in `SignRequest`, which would block the auto-approve flow in tests.

## T093 — Remaining Android tests

- `KeyDetailViewModel` can be instantiated in tests with a mock `KeyManager`, a real `KeyUnlockManager()`, and `SavedStateHandle(mapOf("keyId" to "new"))` for create-mode. The ViewModel's `init` block loads the key from `keyId`, so "new" triggers create-mode without KeyManager calls.
- The None-unlock warning dialog confirm button text is "Disable Unlock" (not "Enable") — the UI text describes the action being taken, not the policy name. Always check the actual Composable source for button labels.
- `JsonTree.log()` calls `android.util.Log.println()` which requires the Android runtime. Structured logging tests must be `androidTest` (instrumented), not plain unit tests, to exercise the real code path.

## T084 — Makefile DX validation

- `make proto` fails if `gen/` directory doesn't exist (e.g., after `make clean-all`). Fix: add `mkdir -p $(GEN_DIR)/nixkey/v1` before `protoc`.
- `make clean-all` with `rm -rf gen/` deletes tracked fuzz test files (`gen/nixkey/v1/fuzz_test.go` and `testdata/`). Fix: use `find gen/ -name '*.pb.go' -delete` to only remove generated protobuf files.

## T085 — Cold-start and idempotency tests

- The control server command for status is `"get-status"`, not `"status"`. The `handleCommand` switch uses `"get-status"`. Using `"status"` returns `{status: "error", error: "unknown command: status"}`.
- Pairing-level tests that need access to unexported functions (`processPairingResult`, `generateClientCertPair`, `ensureAgeIdentity`) must live in the `pairing` package. Go's `export_test.go` pattern only works within the same package's test build, not for external test packages.

## T086 — Host integration tests (hardening)

- `daemon.ControlServer.Stop()` panics on double-call because it calls `close(s.done)` twice. Never use both an explicit `Stop()` call in the test body AND `t.Cleanup(srv.Stop)` — pick one.
- To test `SaveToJSON` write failure, making the parent directory read-only with `os.Chmod(dir, 0500)` is insufficient if the file already exists (existing files remain writable). Instead, try writing to a path inside a non-existent subdirectory under a read-only parent so `os.MkdirAll` fails.
- The PairingServer's one-time token mechanism naturally enforces concurrent pairing rejection: the `tokenUsed` flag is set under a mutex on first use, and subsequent requests with the same token get `401 Unauthorized`. After processing, the server shuts itself down.

## T097 — License determination

- All Go deps (filippo.io/age BSD-3, tailscale BSD-3, spf13/cobra Apache-2.0, gvisor Apache-2.0, OTEL Apache-2.0, golang.org/x BSD-3, grpc Apache-2.0) and Android deps (AndroidX/Compose/Hilt Apache-2.0, BouncyCastle MIT, Protobuf BSD-3) are permissive — MIT is compatible with all of them.
- `go-licenses` is not in the nix devshell; manual verification via `go mod download` + reading LICENSE files in GOMODCACHE works as a fallback.
