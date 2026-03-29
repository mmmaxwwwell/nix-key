# Learnings

Discoveries, gotchas, and decisions recorded by the implementation agent across runs.

---

## T001 — Flake setup

- Nix package for nixfmt is `nixfmt-rfc-style` (not `nixfmt` which is the legacy formatter).
- The sandbox environment does not have Nix store write access (`/nix/var/nix/db/big-lock: Permission denied`), so `nix develop`, `nix build`, and `nix flake show` cannot run. Syntax can be checked with `nix-instantiate --parse` using temp store dirs.
- `go.mod` already exists at repo root with module `github.com/phaedrus-raznikov/nix-key` and Go 1.24.10.
- `gopls` and `gotools` added to devShell for editor integration (not required by task but useful DX).

