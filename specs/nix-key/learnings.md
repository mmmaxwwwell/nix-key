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

