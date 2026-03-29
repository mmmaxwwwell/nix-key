# Attempt 5 — Fix Notes

## Fix applied

Removed `pkgs` from node function destructuring in `nix/tests/pairing-test.nix` at lines 22 and 88.

- Before: `{ config, lib, pkgs, ... }:`
- After: `{ config, lib, ... }:`

This prevents NixOS module system's base `pkgs` from shadowing the outer overlay-augmented `pkgs` that includes `pkgs.nix-key` and `pkgs.phonesim`.

## Local validation

- **Build**: `go build ./...` — passed
- **Lint**: `golangci-lint run` — 0 issues
- **Unit tests**: `go test -short ./...` — all passed
- **Nix syntax check**: Cannot run `nix-instantiate --parse` in sandbox (permission denied on `/nix/var/nix/db/big-lock`) — pre-existing sandbox limitation documented in learnings

## Notes

This was the 5th layer of masked errors in `pairing-test.nix`. Each prior attempt fixed a real issue that was blocking evaluation before this point:
- Attempt 3: DNS assertion for headscale
- Attempt 4: `tls_cert_path` type change (str -> null)
- Attempt 5: `pkgs` shadowing in node functions
