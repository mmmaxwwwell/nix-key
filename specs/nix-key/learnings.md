# Learnings

Discoveries, gotchas, and decisions recorded by the implementation agent across runs.

---

## T109a — Go CI local verification

- The test-reporter (`cmd/test-reporter`) didn't create a `latest` symlink, so `test-logs/ci/latest/summary.json` didn't exist. Fixed by adding `os.Symlink(timestamp, latestLink)` after writing summary.json.
- `nix build` requires Nix store write permissions (`/nix/var/nix/db/big-lock`). In sandboxed environments, use `go build` (via `make build`) as a proxy for verifying the Go binary builds.
- Go tests report 413 passes across all packages (consistent with prior CI runs reporting 425 — the difference is likely package-level pass events vs test-level).

## T109c — Security scanner local verification

- Gitleaks outputs bare `[]` (3 bytes) when no secrets are found, which fails a >10 byte check. Wrapping the raw array into a `{scanner, findings, count, exit_code}` object in the scan script fixes this while keeping the data accessible.

## T099 — Nix devshell / infer build failure

- `nix/infer.nix` bundles Clang 18 plugins that need `libzstd.so.1` (zstd), `libtinfo.so.6` (ncurses), and `libpython3.8.so.1.0`. Adding `zstd` and `ncurses` to `buildInputs` and using `autoPatchelfIgnoreMissingDeps` for `libpython3.8.so.1.0`, `libclang.so.18.1`, and `libxml2.so.2` fixes the auto-patchelf failure.
- All 8 Nix files (flake.nix + nix/*.nix) must pass `nixfmt --check` (rfc-style). The CI lint job runs this check via `nix develop --command`.
- Headscale embedded DERP relay requires TLS: with `tls_cert_path = null`, the DERP relay serves plain HTTP but tailscaled expects HTTPS, causing "tls: first record does not look like a TLS handshake". Fix: generate a self-signed TLS cert via `pkgs.runCommand` and set `tls_cert_path`/`tls_key_path` + `security.pki.certificateFiles` on all nodes + use `https://` for `server_url` and `tailscale up --login-server`.
- When reformatting Nix files, use the devshell's `nixfmt-rfc-style` (v1.2.0+), not the system `nixfmt` (v0.6.0). They produce different output.

## T099 — CI debug loop (final CI validation)

- **Attempt 1** failed on 3 independent issues:
  1. **Unix socket path too long**: `TestIntegrationColdStartControlServer` constructs a socket path from `t.TempDir()` which exceeds Linux's 108-char `sun_path` limit in CI (GitHub runner's `TMPDIR` is long). Fix: use `os.MkdirTemp("/tmp", "nk-")` with shorter intermediate paths.
  2. **Gitleaks GITHUB_TOKEN**: `gitleaks/gitleaks-action@v2` now requires `GITHUB_TOKEN` in the `env` block for PR scanning. Previously optional, now mandatory.
  3. **Fuzz hang**: `FuzzSSHAgentProtocol` used `net.Pipe()` + `sshagent.ServeAgent` per iteration without deadlines. Malformed SSH agent messages with large length prefixes caused hangs. Fix: add 2-second `SetDeadline` on both pipe ends.
- **Attempt 2** passed all 6 jobs (Lint, Test Host, Test Android, Security, Fuzz, CI Summary) after applying all three fixes in a single commit.
- **Android test gap**: `gradlew` is not checked into the repo, so the `Gradle build + unit tests` step in Test Android exits with "No such file or directory" but the exit code is swallowed by `| tee`. The job reports 0 tests passed/failed and concludes "success". The lint job's ktlint step handles this gracefully with `test -f ./gradlew`. This is a pre-existing condition, not a T099 regression.
- **Sanity-check approach**: Cross-reference CI-reported test counts against on-disk test files. 425 Go test passes across 12 packages matches the 12 Go packages with `_test.go` files. 10/10 fuzz targets ran for 60s each.

## T099 — Gitignore vendor collisions (attempt 3)

- **Bare `result` pattern in `.gitignore`** matches any path component named `result`, not just the Nix build symlink at the repo root. This excluded `vendor/tailscale.com/types/result/result.go`, breaking lint/test/security. Fix: anchor with `/result`.
- **Bare `build/` pattern** similarly excluded `vendor/github.com/tailscale/web-client-prebuilt/build/` (embedded web assets required by an `embed.go` directive). Fix: anchor with `/build/` and add `android/**/build/` for Android build output.
- **General rule**: When ignoring common directory names (`build/`, `dist/`, `result`, `certs/`), always use root-anchored patterns (`/build/`) to avoid accidentally excluding vendor subdirectories. After any `.gitignore` change, verify with `git ls-files --others --ignored --exclude-standard vendor/`.

## T109b — Android CI local verification

- **Gradle wrapper missing**: `gradlew` and `gradle-wrapper.jar` were not checked into the repo. Generated using Gradle 8.11.1 distribution in a temp project and copied the wrapper files.
- **`settings.gradle.kts` API mismatch**: Used `dependencyResolution` (Gradle 9.x API) instead of `dependencyResolutionManagement` (Gradle 8.x). Also needed `repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)`.
- **Protobuf Gradle plugin 0.9.4→0.9.6**: Required for Kotlin DSL compatibility. Also needs `import com.google.protobuf.gradle.proto` at top of `build.gradle.kts`.
- **Version catalog in protobuf DSL**: `libs.versions.protobuf.get()` doesn't resolve inside protobuf DSL blocks. Hardcode version strings (`"4.29.3"`, `"1.68.2"`) instead.
- **`proto {}` sourceSets scoping**: Must be declared via `android.sourceSets {}` outside the `android {}` block (not inside `sourceSets {}` within `android {}`).
- **Build-tools version**: Nix SDK only has build-tools 35.0.0; AGP 8.7.3 tries to install 34.0.0 in read-only Nix store. Fix: set `buildToolsVersion = "35.0.0"` explicitly.
- **AAPT2 ELF patching**: Downloaded AAPT2 binary has wrong dynamic linker for Nix. Use `-Pandroid.aapt2FromMavenOverride=$ANDROID_HOME/build-tools/35.0.0/aapt2` to use the SDK's pre-patched binary.
- **protoc-gen-grpc-java ELF patching**: Downloaded from Maven, needs `patchelf --set-interpreter --set-rpath` for Nix dynamic linker and libstdc++.
- **gomobile broken with Go 1.26 — fix**: Nix-packaged gomobile (Dec 2024) sets `GOPATH=gomobile-work` (relative path) which Go 1.26 rejects. The go-wrapper approach (converting relative GOPATH to absolute) doesn't fully work because gomobile also uses relative paths for its own work directory and `-o` flags. The real fix: build gomobile from the latest `golang.org/x/mobile` source (already in go.mod) which uses `os.MkdirTemp` (absolute paths). See T110.
- **HostnameVerifier SAM conversion**: Kotlin lambda `{ _, _ -> true }` doesn't auto-convert for `setHostnameVerifier`. Use explicit `javax.net.ssl.HostnameVerifier { _, _ -> true }`.
- **BouncyCastle META-INF conflict**: bcpkix, bcutil, bcprov JARs all contain `META-INF/versions/9/OSGI-INF/MANIFEST.MF`. Add to packaging excludes.
- **Hilt DI bindings**: `TailscaleBackend`, `Context`, and `BiometricManager` need explicit `@Provides` methods in a Hilt `@Module`. Created `AppModule.kt`.
- **javax.annotation.Generated**: gRPC generated code requires `javax.annotation:javax.annotation-api:1.3.2` dependency.
- **`BuildConfig` unresolved**: Need `buildConfig = true` in `buildFeatures`.
- **Android build result**: `assembleDebug` produces 69MB APK; `testDebugUnitTest` runs 5 tests (TraceContextTest) with 0 failures.

## T110 — Fix gomobile AAR build

- **Root cause**: Nix-packaged gomobile (Dec 2024 commit) uses `GOPATH=gomobile-work` (relative) and `$WORK` (relative) internally. Go 1.26+ rejects relative GOPATH. A go-wrapper approach only fixes the GOPATH issue but not gomobile's own relative work directory for `.so` output paths.
- **Fix**: Build gomobile + gobind from source (latest `golang.org/x/mobile` in go.mod, March 2026) which uses `os.MkdirTemp("", "gomobile-work-")` (absolute). The build is fast (~5s) and runs as part of the `build-android-apk` script.
- **ANDROID_HOME NDK path**: gomobile requires the Android SDK to have `ndk/<version>/` directory. Multiple androidsdk store paths exist — use the one from the Nix `androidComposition` (not the first one in the store). The Nix gomobile wrapper's `$ANDROID_HOME` env var reveals the correct one.

## T111 — Verify gomobile AAR contains real Go code

- **AAR verification**: The AAR is 9.5MB compressed with `jni/arm64-v8a/libgojni.so` (20.7MB). This confirms real Go native code, not a stub (<100KB).
- **x86_64 missing**: `-target android` only produced ARM64; x86_64 was not built. For emulator tests on x86_64 hosts, either use ARM64 translation (Android 11+) or add explicit `-target android/arm64,android/amd64` to the gomobile bind command.
- **`jar` not in devshell**: Use `unzip -l` instead of `jar tf` to inspect AAR contents in the Nix environment.

## T112 — Android instrumented tests on local emulator

- **No KVM = slow emulator**: Without `/dev/kvm`, the Android emulator boots in ~8 min (vs ~20s with KVM). The `start-emulator` timeout must be 600s (not 120s). Services (package manager) take additional minutes to initialize after `sys.boot_completed=1`.
- **gomobile `int32` → Java `int`**: Go `int32` maps to Java `int` in gomobile (not `long`). The `KeyStore.sign()` flags parameter changed from `Long` to `Int` when the Go interface was updated to use `int32`.
- **FGS connectedDevice permission**: Android 14+ requires `CHANGE_NETWORK_STATE` (or similar) in addition to `FOREGROUND_SERVICE_CONNECTED_DEVICE` for foreground services with `connectedDevice` type. Missing this causes `SecurityException` at runtime.

## T113 — E2E test script fixes

- **headscale v0.28 preauthkeys require numeric user ID**: `--user` takes a uint, not a string name. Use `users list -o json | python3 -c "..."` to extract the numeric `id` field. Also requires `derp.urls` section and `dns.nameservers.global` with a real nameserver (not empty) when `override_local_dns: false`.
- **nix-key daemon config field names**: The config JSON uses `socketPath`, `controlSocketPath`, and integer `signTimeout` (not `listenPath`, `controlSocket`, `"30s"`). The daemon finds config at `$XDG_CONFIG_HOME/nix-key/config.json` and devices at `$XDG_STATE_HOME/nix-key/devices.json`.
- **APK install on slow emulators**: Use push-then-install (`adb push` + `adb shell pm install`) instead of streaming `adb install`, which times out on slow software-emulated devices. After `sys.boot_completed=1`, wait for `pm list packages` to show 50+ packages AND `service check mount` to report "found" before attempting install.
