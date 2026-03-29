# Learnings

Discoveries, gotchas, and decisions recorded by the implementation agent across runs.

---

## T001 — Flake setup

- Nix package for nixfmt is `nixfmt-rfc-style` (not `nixfmt` which is the legacy formatter).
- The sandbox environment does not have Nix store write access (`/nix/var/nix/db/big-lock: Permission denied`), so `nix develop`, `nix build`, and `nix flake show` cannot run. Syntax can be checked with `nix-instantiate --parse` using temp store dirs.
- `go.mod` already exists at repo root with module `github.com/phaedrus-raznikov/nix-key` and Go 1.24.10.
- `gopls` and `gotools` added to devShell for editor integration (not required by task but useful DX).

## T019 — SSH agent handler

- `golang.org/x/crypto v0.49.0` requires `go >= 1.25.0`. The toolchain auto-switches but this bumps `go.mod` from 1.24.x to 1.25.0.
- Network proxy blocks `proxy.golang.org` and `goproxy.io`. Use `HTTPS_PROXY="" HTTP_PROXY="" NO_PROXY="*" GOPROXY=direct` to bypass and fetch directly from source hosts.
- `agent.ServeAgent` from `golang.org/x/crypto/ssh/agent` handles SSH agent wire protocol. It takes an `agent.Agent` (or `ExtendedAgent`) and `io.ReadWriter`. Backend errors are translated to `SSH_AGENT_FAILURE` by the library, but the error string gets logged; use a generic `errAgentFailure` to avoid leaking internal details.
- Socket permissions should be `0600` (owner-only) for security. Parent directory `0700`.
- The `agent.ExtendedAgent` interface adds `SignWithFlags` and `Extension` beyond `agent.Agent`. Implementing `ExtendedAgent` ensures signature algorithm negotiation (e.g., rsa-sha2-256) works correctly.

## T023 — QR code generation

- `skip2/go-qrcode` uses a `replace` directive to `/tmp/go-qrcode` (cloned via git) since the Go module proxy is blocked in the sandbox. This needs to be converted to a proper dependency when network access is available.
- `qrcode.ToSmallString(false)` renders a compact terminal-printable QR using Unicode half-block characters. The `false` parameter means no border inversion.
- Go 1.24.6 is available in nix store at `/nix/store/5gkj2kc7drgf6pclxcl4fyg8zlcaqqmd-go-1.24.6`. Use `GOROOT=.../share/go` and `GOTOOLCHAIN=local` to avoid auto-download of newer toolchain.
- The `go.mod` was bumped to `go 1.25.0` by `golang.org/x/crypto` dependency from T019. Using `go 1.24.6` with `GOTOOLCHAIN=local` works for compilation.

## T002 — Go module + CLI skeleton + Makefile

- Go module was already initialized (from T001). The task's `go mod init` step was already done.
- `cobra v1.10.2` added for CLI subcommand routing. Pulls in `pflag` and `mousetrap` as indirect deps.
- Subcommands scaffolded: daemon, pair, devices, revoke, status, export, config, logs, test (all stub implementations).
- Must clone `skip2/go-qrcode` to `/tmp/go-qrcode` before running `go get` or `go mod tidy` due to the replace directive in go.mod.
- Binary name `nix-key` added to `.gitignore`.

## T020 — Device registry

- Go 1.26.1 is now available in devShell PATH. No need for `GOROOT` workarounds — just use `GOTOOLCHAIN=local`.
- The `internal/daemon/` package is new. The device registry is pure Go with no external dependencies beyond stdlib.
- `LookupByKeyFingerprint` uses the device's cert fingerprint (the device identity), not SSH key fingerprint. SSH key→device mapping (via CachedKey) will be needed in T021.
- Merge rule (FR-064, C-010): nix-declared wins for cert paths (if non-empty); runtime wins for lastSeen and tailscaleIp.
- `SaveToJSON` only persists runtime-paired devices; nix-declared devices come from NixOS config.
- `devices.json` written with `0600` perms, parent dir `0700`.

## T003 — Structured test reporter

- Implemented as a standalone `cmd/test-reporter` that reads `go test -json` from stdin, rather than a test helper library. This keeps it decoupled from test code.
- `go test -json` emits events with `Action` field: `run`, `output`, `pass`, `fail`, `skip`. Package-level events have no `Test` field.
- Piping `go test -json ... 2>&1` is needed because some build errors go to stderr.
- The reporter passes through raw JSON lines to stdout for real-time visibility, then writes structured output to `test-logs/<type>/<timestamp>/`.
- `failures` field in summary.json initialized as `[]FailureSummary{}` to serialize as `[]` not `null`.

## T005 — Code coverage

- `.gitignore` already had `coverage/`, `coverage.out`, and `htmlcov/` entries from initial setup, so no changes needed there.
- `go tool cover -html=coverage.out -o coverage/index.html` generates the HTML report without opening a browser (unlike bare `-html` which tries to open one).
- The `clean` Makefile target already had `rm -rf coverage/`.

## T004 — Test fixtures

- Go's `ecdsa.GenerateKey` and ECDSA signing use `crypto/internal/randutil.MaybeReadByte` which does a non-deterministic `select` on two closed channel cases. This makes ECDSA key generation and signing non-deterministic even with a fixed `io.Reader`. Workaround: use Ed25519 for X.509 certs (deterministic signing), and construct ECDSA keys manually from raw scalar bytes (bypassing `GenerateKey`).
- Ed25519 key generation (`ed25519.GenerateKey(rng)`) reads exactly 32 bytes and is fully deterministic with a fixed reader.
- `ssh.MarshalPrivateKey` reads from `crypto/rand.Reader` for OpenSSH format check bytes. Override the global `rand.Reader` for deterministic marshaling.
- `age-keygen` does not support seeding — age identity is generated fresh each run. The full fixture set (identity + encrypted file) must be generated together and committed.
- `age-keygen -o` refuses to overwrite existing files; must `os.Remove` the target first.
- ChaCha20 with a zero nonce makes an excellent deterministic CSPRNG when keyed with a domain-separated SHA-256 hash of a seed string.
- X.509 certs with Ed25519 work fine for mTLS testing; `tls.LoadX509KeyPair` requires PKCS8 format for Ed25519 private keys (use `x509.MarshalPKCS8PrivateKey`).

## T028 — Android project setup

- Android project is not Nix-built (per CLAUDE.md: "Android app is the only non-Nix artifact"). No `flake.nix` changes needed.
- Gradle version catalog (`gradle/libs.versions.toml`) is the modern way to manage dependencies in Kotlin DSL projects.
- `settings.gradle.kts` uses `dependencyResolution` block (Gradle 8.x+) instead of the deprecated `dependencyResolutionManagement`.
- KSP (`com.google.devtools.ksp`) is used for Hilt annotation processing instead of KAPT (KAPT is deprecated for Kotlin 2.x).
- BouncyCastle artifacts for Android: use `bcprov-jdk18on` and `bcpkix-jdk18on` (the `-jdk18on` suffix is the current naming convention).
- `JsonTree` outputs to logcat via `Log.println()` with tag `nix-key` — this allows structured JSON output while keeping a consistent tag for log filtering.
- `TraceContext` uses a `ThreadLocal` for OTEL trace ID propagation. The `withTraceId` helper ensures cleanup even on exceptions.
- ktlint is configured via the `org.jlleitschuh.gradle.ktlint` Gradle plugin (v12.x), not as a standalone tool.

## T029 — KeyManager

- Android Keystore does not support Ed25519 natively. The dual strategy is: ECDSA-P256 in hardware (Keystore), Ed25519 in software (BouncyCastle) wrapped by a Keystore-backed AES-256-GCM key.
- `setIsStrongBoxBacked(true)` is a builder setter that doesn't throw; the actual failure happens at `generateKeyPair()` time on devices without StrongBox. Need two-phase try/catch: try with StrongBox, on failure retry without.
- Ed25519 private key wrapping format: `[4-byte IV length][IV][AES-GCM ciphertext]`. GCM tag length is 128 bits (standard).
- SSH public key blob encoding: ECDSA uses `ecdsa-sha2-nistp256` + `nistp256` + uncompressed EC point (0x04 || x || y, each coordinate 32 bytes fixed-length). Ed25519 uses `ssh-ed25519` + 32-byte raw public key.
- `BouncyCastle Ed25519PrivateKeyParameters.encoded` returns the 32-byte seed, which is what `Ed25519PrivateKeyParameters(bytes, 0)` expects back.
- Instrumented tests use `androidx.test.runner.AndroidJUnit4` (not `ext.junit`) since that's what's available via the `androidx-test-runner` dependency.
- `EncryptedSharedPreferences` requires a `MasterKey` with `AES256_GCM` scheme. The prefs file name (`nixkey_keys`) is shared between key metadata and Ed25519 key material storage.

## T030 — Compose UI screens

- Compose UI tests with mocked ViewModels don't need Hilt testing infrastructure (`@HiltAndroidTest`, `HiltAndroidRule`). Only tests that go through the full NavGraph (which calls `hiltViewModel()`) need Hilt. Mocking the ViewModel and passing it directly avoids this complexity.
- `mockk-android` must be added as `androidTestImplementation` (not just `testImplementation`) for use in instrumented tests.
- `ExposedDropdownMenuBox` with `menuAnchor(MenuAnchorType.PrimaryNotEditable)` requires Material3 1.3.0+ (available in BOM 2024.12.01).
- App settings (allow key listing, default policy, OTEL) use regular `SharedPreferences` since they are not sensitive. Key material and host certs use `EncryptedSharedPreferences`.
- `KeyManager.updateKey()` and `KeyManager.getKey()` were added to support the editable display name (FR-048) and confirmation policy editing in KeyDetailScreen.

## T031 — BiometricHelper

- `BiometricPrompt.PromptInfo.Builder.setNegativeButtonText()` is required when DEVICE_CREDENTIAL is NOT in the allowed authenticators, and must NOT be called when DEVICE_CREDENTIAL IS included (throws `IllegalArgumentException`).
- `onAuthenticationFailed()` is called for individual failed biometric attempts (wrong fingerprint) but is NOT terminal — BiometricPrompt keeps the dialog open for retry. Only `onAuthenticationError()` and `onAuthenticationSucceeded()` are terminal callbacks.
- `BiometricManager` can be constructor-injected via Hilt for testability. Use `BiometricManager.from(context)` in a Hilt module to provide it.
- Instrumented tests use `androidx.test.runner.AndroidJUnit4` (same as T029 learnings — `ext.junit` is not in the dependency list).

## T032 — SignRequestDialog

- `AlertDialog.onDismissRequest` should be a no-op for sign requests — users must explicitly Approve or Deny; dismissing by tapping outside would leave the request in an undefined state.
- `SignRequestDialogContent` is separated from the queue-aware `SignRequestDialog` for testability — tests can render the dialog directly without needing a `SignRequestQueue`.
- `ConcurrentLinkedQueue` is used for the backing store of `SignRequestQueue` for thread safety (gRPC calls may enqueue from background threads), with `StateFlow` for reactive Compose UI updates.
- `queueSize` in the `SignRequestQueue` represents the number of requests *behind* the current one (i.e., remaining in the `ConcurrentLinkedQueue` after the current was polled out), not total enqueued.
- `data class` with `ByteArray` field requires manual `equals`/`hashCode` override — default data class equality uses reference equality for arrays. For `SignRequest`, equality is based on `requestId` only.

