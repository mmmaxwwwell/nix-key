# Learnings

Discoveries, gotchas, and decisions recorded by the implementation agent across runs.

---

## T097 — License determination

- All Go deps (filippo.io/age BSD-3, tailscale BSD-3, spf13/cobra Apache-2.0, gvisor Apache-2.0, OTEL Apache-2.0, golang.org/x BSD-3, grpc Apache-2.0) and Android deps (AndroidX/Compose/Hilt Apache-2.0, BouncyCastle MIT, Protobuf BSD-3) are permissive — MIT is compatible with all of them.
- `go-licenses` is not in the nix devshell; manual verification via `go mod download` + reading LICENSE files in GOMODCACHE works as a fallback.

## T099 — Nix devshell / infer build failure

- `nix/infer.nix` bundles Clang 18 plugins that need `libzstd.so.1` (zstd), `libtinfo.so.6` (ncurses), and `libpython3.8.so.1.0`. Adding `zstd` and `ncurses` to `buildInputs` and using `autoPatchelfIgnoreMissingDeps` for `libpython3.8.so.1.0` fixes the auto-patchelf failure.
- All 8 Nix files (flake.nix + nix/*.nix) must pass `nixfmt --check` (rfc-style). The CI lint job runs this check via `nix develop --command`.
