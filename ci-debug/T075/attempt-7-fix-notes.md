# Attempt 7 — Fix Notes

## Diagnosis accuracy

The diagnosis correctly identified that `pkgs.jaeger-all-in-one` is missing from the locked
nixpkgs (rev `46db2e09e1d3f113a13c0d7b81e2f221c63b8ce9`). However, its recommended fix to use
`pkgs.jaeger` is **incorrect** — the `jaeger` attribute also does not exist in this nixpkgs
revision. All jaeger-related packages were completely removed from nixpkgs, not just renamed.

Verified via nix-instantiate eval:
- `builtins.hasAttr "jaeger" pkgs` → `false`
- `builtins.hasAttr "jaeger-all-in-one" pkgs` → `false`
- Filtering all attr names matching `.*aeger.*` → `[]`

## Applied fix

Since no jaeger package exists in the locked nixpkgs:

1. **Created `nix/jaeger.nix`**: Fetches the pre-built Jaeger v2.16.0 binary from the official
   GitHub release. Jaeger v2 consolidates all components into a single `jaeger` binary that
   defaults to all-in-one mode (OTLP gRPC on 4317, query UI on 16686).

2. **Added `jaeger` to the flake overlay** (`flake.nix`): `final.callPackage ./nix/jaeger.nix {}`

3. **Added `tracing.jaeger.package` option** to `nix/module.nix`: Defaults to `pkgs.jaeger`
   (from the overlay), users can override.

4. **Updated ExecStart** in module.nix: Changed from
   `${pkgs.jaeger-all-in-one}/bin/jaeger-all-in-one` to `${cfg.tracing.jaeger.package}/bin/jaeger`

5. **Kept the systemd service name as `jaeger-all-in-one`**: Both test files
   (`jaeger-test.nix`, `tracing-e2e-test.nix`) reference `jaeger-all-in-one.service`, so the
   service name is preserved to avoid cascading changes.

## Potential risk

- The `tracing-e2e-test.nix` check (line 130 in flake.nix) may reveal further evaluation
  errors if it has issues beyond the jaeger package reference.
- The `autoPatchelfHook` in `jaeger.nix` is needed because the Jaeger binary is a statically
  compiled Go binary — it should be fine without dynamic libraries, but autoPatchelfHook
  handles edge cases with CGO.

## Local validation

- `go build ./...` — OK
- `golangci-lint run ./...` — 0 issues
- `go test -short ./...` — all pass
- `nix-instantiate --parse` — all .nix files parse correctly
