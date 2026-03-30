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
