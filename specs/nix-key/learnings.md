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

## T033 — gomobile bridge

- gomobile maps Go `int` to Java `long` — need `.toInt()` conversion for port number in Kotlin.
- gomobile exports constructors like `Phoneserver.newPhoneServer(ks, conf)` (package-level factory function) and `Phoneserver.newKeyList()` — these are static methods on the package class.
- The `ConfirmerAdapter` uses `CountDownLatch` to block the Go gRPC thread until the user responds in the Compose UI. The observer pattern (`ConfirmationObserver`) bridges between the UI thread (which calls `notifyCompletion`) and the blocked gRPC thread.
- The `KeyStoreAdapter.sign()` method receives nullable `String?`/`ByteArray?` from gomobile — Go strings/slices become nullable Java types. Must null-check before delegating to `KeyManager`.
- For integration tests, the sign request auto-approve pattern is: spawn a background thread that polls `signRequestQueue.currentRequest`, then calls both `signRequestQueue.complete()` and `confirmerAdapter.notifyCompletion()` to unblock the Go side.
- The `fileTree` dependency in `build.gradle.kts` (`implementation(fileTree(mapOf("dir" to "libs", "include" to listOf("*.aar"))))`) picks up the gomobile AAR from `android/app/libs/phoneserver.aar`.
- Proto stubs for Android are generated by the `protobuf-gradle-plugin` at build time (not gomobile), using proto files from `../../proto`.

## T034 — TailscaleManager

- `libtailscale` is a gomobile-generated AAR (like `phoneserver`). The `TailscaleBackend` interface abstracts the native calls so `TailscaleManager` can be tested with a `FakeTailscaleBackend`.
- Auth key is stored in `EncryptedSharedPreferences` (file: `nixkey_tailscale`). To clear encrypted prefs in tests, use the `TailscaleManager.clearAuthKey()` method rather than raw `context.getSharedPreferences(...).clear()`, because `EncryptedSharedPreferences` uses a different internal storage format.
- `TailscaleManager` uses `AtomicBoolean` and `AtomicReference` for thread safety since Tailscale operations may be called from different threads (UI thread for start/stop, gRPC threads for IP lookup).
- No new dependencies needed — `EncryptedSharedPreferences`, Timber, and Hilt are already in `build.gradle.kts` from prior tasks.

## T035 — GrpcServerService

- Android API 34+ requires `FOREGROUND_SERVICE_CONNECTED_DEVICE` permission and `foregroundServiceType="connectedDevice"` in the manifest for foreground services that communicate with external devices over the network.
- The default gRPC listen port is `29418` (from data-model.md). The constant lives in `SettingsRepository.DEFAULT_LISTEN_PORT` to avoid circular coupling between the service and repository.
- `MainActivity.onStart()`/`onStop()` map to the app being foregrounded/backgrounded. These lifecycle callbacks are used to start/stop the `GrpcServerService` via `startForegroundService()`.
- `startForeground()` must be called in `onStartCommand()` before returning (Android enforces a ~10s deadline). Building the notification channel in `onCreate()` and calling `startForeground()` immediately in `onStartCommand()` satisfies this.
- Hilt injection into Android `Service` classes requires `@AndroidEntryPoint` annotation, same as Activities.

## T039 — NixOS module

- The `nix/` directory does not exist initially; it must be created.
- `nix-instantiate --parse` requires temp store dir workarounds in sandbox (same as T001 learning).
- `configFile` (was `configJson`) is a `pkgs.writeText` derivation in the `let` block — creates the config.json in the Nix store, symlinked into `~/.config/nix-key/` by the service's `preStart`.
- The `assertion` that jaeger.enable and otelEndpoint are mutually exclusive prevents conflicting config; T051 will set otelEndpoint automatically when jaeger is enabled.

## T040 — systemd user service

- `systemd.tmpfiles.rules` are system-level; `%h` resolves to root's home, NOT the invoking user. For user service directories, use `ConfigurationDirectory` and `StateDirectory` in `serviceConfig` instead — systemd sets `$CONFIGURATION_DIRECTORY` and `$STATE_DIRECTORY` env vars pointing to the created paths.

## T041 — Nix package derivation

- `lib.fileset.toSource` is the clean way to specify which source paths go into the derivation — avoids pulling in `.git`, `android/`, `test/`, etc.
- `vendorHash` can be computed locally with `go mod vendor && nix-hash --type sha256 --sri vendor/` when nix-build is unavailable in the sandbox.
- The `gen/` directory (generated proto stubs) must be included in the fileset since `pkg/phoneserver/` imports from `gen/nixkey/v1/`.
- `go mod tidy` promoted `google.golang.org/grpc` and `google.golang.org/protobuf` from indirect to direct deps because `pkg/phoneserver/` directly imports them.
- `go.sum` was missing the `h1:` hash for `skip2/go-qrcode` (only had `.mod` hash); `go mod tidy` fixed this.
- `preStart` in NixOS becomes an `ExecStartPre` wrapper script. Systemd specifiers (`%h`, `%S`) are NOT expanded inside shell script content — use `$HOME`, `$STATE_DIRECTORY`, `$CONFIGURATION_DIRECTORY` env vars instead.
- `environment.etc."xdg/environment.d/50-nix-key.conf"` places the file at `/etc/xdg/environment.d/50-nix-key.conf` — this is the system-wide XDG default directory, picked up by `systemd --user` for all users' login sessions.
- `lib.getExe` requires the package to have `meta.mainProgram` set or a single output binary. T041 (package.nix) must ensure this.

## T042 — Flake exports + default.nix

- `nixosModules` and `overlays` are system-independent flake outputs — they go outside `eachDefaultSystem`. Merge with `//` operator.
- `pkgs` should be constructed with `import nixpkgs { inherit system; overlays = [...]; }` (not `legacyPackages`) when applying the overlay, so `pkgs.nix-key` is available.
- `checks` for NixOS VM tests are only meaningful on `x86_64-linux`; use `nixpkgs.lib.optionalAttrs` to gate them.
- `builtins.pathExists` works at eval time to conditionally include test files that don't exist yet (e.g., `nix/tests/service-test.nix`).
- `default.nix` provides non-flake import by returning `{ package, module, overlay }` attrset from `pkgs.callPackage`.

## T007 — Structured JSON logger

- `slog.NewJSONHandler` uses `time` as the default timestamp key; use `ReplaceAttr` to rename it to `timestamp` for spec compliance.
- `slog.LevelVar` allows setting the level dynamically and is the recommended way to pass a level to `HandlerOptions.Level`.
- `WithModule` is just `logger.With("module", name)` — slog's `With` creates a new logger that includes the attrs in every subsequent log call without mutating the parent.

## T008 — Error hierarchy

- Go's `errors.As` walks the chain via `Unwrap()`. Embedding `NixKeyError` in subtypes means `errors.As(err, &nkErr)` needs a custom `As` method on each subtype to convert `**NixKeyError` target to the embedded field.
- Sentinel errors (`ErrConnection`, `ErrTimeout`, etc.) combined with `Is(target error) bool` methods on each subtype allow `errors.Is(err, ErrConnection)` pattern matching without needing type assertions.
- `CodeFrom(err)` uses an interface (`Code() string`) with `errors.As` to extract codes from anywhere in the error chain, including when wrapped by `fmt.Errorf("%w", ...)`.

## T009 — Config module

- Config struct uses `json:"fieldName"` tags with camelCase JSON keys matching the NixOS module's `config.json` output.
- `json.Unmarshal` into a pre-populated struct (defaults) correctly overlays only the fields present in the JSON file, leaving defaults for absent fields.
- `bool` fields in JSON unmarshal to `false` when absent from the file (Go zero value), which conflicts with `allowKeyListing` defaulting to `true`. Solved by setting the default before unmarshal — if the JSON file explicitly sets it to `false`, that's intentional; if absent, the default `true` from the pre-populated struct is preserved.
- Optional nullable fields (`otelEndpoint`, `tailscaleAuthKeyFile`) use `*string` — `null` in JSON maps to Go `nil`, and env var overrides create a non-nil pointer.
- `IsConfigError` helper added to `internal/errors/` for cleaner test assertions.
- Env var naming convention: `NIXKEY_` prefix + `SCREAMING_SNAKE_CASE` (e.g., `NIXKEY_SIGN_TIMEOUT`, `NIXKEY_ALLOW_KEY_LISTING`).

## T043 — NixOS VM service test

- In Nix `''...''` strings, `$(...)` (shell subshell syntax) causes parse errors — Nix treats `$` followed by `(` as an invalid token. Use `find ... | xargs cat` pipelines instead of `cat $(find ...)`.
- NixOS VM tests access user services via `systemctl --user -M testuser@` (machinectl transport). Requires the user to have lingering enabled (`/var/lib/systemd/linger/<user>`) so the user manager starts at boot without a login session.
- `ConfigurationDirectory` and `StateDirectory` in systemd user services create dirs under `~/.config/` and `~/.local/state/` respectively (not under `/etc/` or `/var/`).
- `pkgs.writeText` generates a store path like `/nix/store/<hash>-nix-key-config.json`. The test finds it with `find /nix/store -maxdepth 1 -name '...'`.
- `environment.etc."xdg/environment.d/50-nix-key.conf"` places the file at `/etc/xdg/environment.d/50-nix-key.conf` — the system-wide XDG defaults directory.

## T044 — Device merge VM test

- To write files to the VM as a specific user, it's simplest to write as root and `chown` — avoids heredoc quoting issues inside `su -c`.
- Python's `json.dumps()` can be used inline in the NixOS test script to construct JSON data, which is then passed to `printf '%s' '...'` in a shell command.
- The config.json symlink to the Nix store is inherently read-only, which structurally prevents CLI tools from modifying Nix-declared devices — this is the mechanism that ensures Nix-declared devices can only be removed by Nix rebuild.
- `import json as json_mod` is needed in the test script if `json` is already used as a variable name earlier (shadowed by the top-level `import json`). Actually, `json` module is imported at the top; re-importing with an alias avoids any confusion.

## T010 — Graceful shutdown

- `sync.Once` ensures shutdown runs exactly once even if called concurrently or multiple times. The error from the first call is captured via a closure variable.
- `sync.WaitGroup` is used for in-flight request tracking. `AddInFlight()`/`DoneInFlight()` are the public API; the drain phase does `inFlight.Wait()` in a goroutine with a deadline select.
- The shutdown sequence is: stopFunc (stop accepting) -> drain in-flight (with deadline) -> hooks in reverse order. If draining times out, hooks are skipped and timeout error is returned.
- `os/signal.Notify` with a buffered channel (cap 1) ensures the signal is not lost if the goroutine hasn't entered the select yet.
- `Run()` passes `context.Background()` to `Shutdown()` rather than the cancelled parent context, so the shutdown deadline starts fresh.

## T036 — Android pairing screen

- Renaming `PairedHost.name` to `PairedHost.hostName` aligns with the data model but requires updating all consumers: `ServerListScreen`, `NavigationTest`, and `HostRepository` storage keys.
- `LocalLifecycleOwner` moved from `androidx.compose.ui.platform` to `androidx.lifecycle.compose` in lifecycle 2.8+. Use the new import to avoid deprecation.
- The `lifecycle-runtime-compose` dependency is needed for `LocalLifecycleOwner` from `androidx.lifecycle.compose`.
- ML Kit barcode scanning requires CameraX (`camera-core`, `camera-camera2`, `camera-lifecycle`, `camera-view`) to feed video frames for analysis.
- `@ExperimentalGetImage` from `androidx.camera.core` is required when calling `imageProxy.image` (the `getImage()` method). Use `@androidx.annotation.OptIn(ExperimentalGetImage::class)` on the composable.
- QR payload is Base64-encoded JSON: `{v:1, host, port, cert, token, otel?}`. Use `android.util.Base64.decode(rawValue, Base64.DEFAULT)` to decode.
- `EncryptedSharedPreferences` stores each `PairedHost` field as a separate key-value pair prefixed with the host ID. This pattern supports multiple hosts (FR-030) while allowing individual field retrieval.
- The host pairing endpoint path is `/pair` and expects a POST with JSON body `{phoneName, tailscaleIp, listenPort, serverCert, token}`. Response is `{hostName, hostClientCert, status}`.
- `HttpsURLConnection` with `hostnameVerifier = { _, _ -> true }` and a trust-all SSL context is needed for connecting to the host's temporary self-signed HTTPS pairing server. The QR payload contains the cert for verification in production, but hostname verification is skipped since Tailscale IPs are used.
- Set `readTimeout = 120_000` on the pairing HTTPS connection because the host holds the connection open until the user confirms the pairing on the CLI side.

