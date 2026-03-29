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

