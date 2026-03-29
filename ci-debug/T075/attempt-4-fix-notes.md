# Attempt 4 — Fix Notes

## Changes made

1. **`nix/tests/pairing-test.nix` (lines 50-51)**: Changed `tls_cert_path = "";` and `tls_key_path = "";` to `tls_cert_path = null;` and `tls_key_path = null;`. The upstream nixpkgs headscale module now types these as `null or absolute path`, rejecting empty strings.

2. **`.github/workflows/ci.yml` (lines 41-44)**: Made ktlint step conditional — checks for `./gradlew` existence before running. The `android/gradlew` wrapper was never committed, so the step would always fail. This is Option B from the diagnosis (skip when not ready) since the Android app isn't ready for CI yet.

## Local validation

- `go build ./...` — PASS
- `golangci-lint run ./...` — PASS (0 issues)
- `go test -short ./...` — PASS (all packages)
- Nix syntax: could not run `nix-instantiate --parse` due to sandbox `/nix/var/nix/db/big-lock` restriction (pre-existing, documented in learnings)
