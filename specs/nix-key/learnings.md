# Learnings

Discoveries, gotchas, and decisions recorded by the implementation agent across runs.

---

## T097 — License determination

- All Go deps (filippo.io/age BSD-3, tailscale BSD-3, spf13/cobra Apache-2.0, gvisor Apache-2.0, OTEL Apache-2.0, golang.org/x BSD-3, grpc Apache-2.0) and Android deps (AndroidX/Compose/Hilt Apache-2.0, BouncyCastle MIT, Protobuf BSD-3) are permissive — MIT is compatible with all of them.
- `go-licenses` is not in the nix devshell; manual verification via `go mod download` + reading LICENSE files in GOMODCACHE works as a fallback.

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

## T105 — Verify scanners ran

- Security scanner JSON outputs are in `test-logs/security/{trivy,semgrep,gitleaks,govulncheck}.json`. The "Verify scanners ran" step should be placed after all scanner steps but before "Generate security summary JSON" to ensure all outputs are available.
- Scanner verification uses `::warning::` (not `::error::`) because missing scanners are advisory — some scanners (e.g., semgrep, gitleaks) use `continue-on-error: true` and may not produce output in all environments.
