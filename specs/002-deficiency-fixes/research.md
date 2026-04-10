# Research: 002-deficiency-fixes

## Decisions

### Struct tag validation library (T009)
**Decision**: Use `github.com/go-playground/validator/v10`
**Rationale**: Most widely used Go struct tag validation library. Already used in many enterprise Go projects. Supports required, min, max, oneof, and custom validators. Zero-config — just add struct tags and call `validate.Struct()`.
**Rejected**: `go-ozzo/validation` (function-based, not tag-based — doesn't satisfy the spec requirement for struct tags), hand-rolled tag parsing (unnecessary complexity).

### Shutdown logging approach (T010)
**Decision**: Inject `*slog.Logger` into `ShutdownManager` via constructor. Add structured log messages at each shutdown step.
**Rationale**: The project already uses `slog` everywhere via `internal/logging/`. Injecting the logger follows the existing dependency injection pattern (no global mutable state per coding standards). Log flushing via the slog handler's `Close()` or a sync function passed to the shutdown manager.
**Rejected**: Using a global logger (violates project coding standards), logging only at start/end (spec requires each step).

### TailscaleBackend implementation (T034)
**Decision**: Wrap `tsnet.Server` (from `tailscale.com/tsnet`) via gomobile bridge in a new `pkg/tsbridge/` package. The gomobile AAR already builds (T110-T111 verified). Go `Start(authKey, dataDir string) (string, error)` maps to Kotlin's existing `fun start(authKey: String?, dataDir: String): String?` — gomobile automatically converts Go errors to Kotlin exceptions and Go `(string, error)` to `String?` with exception on error. The Kotlin side calls the Go bridge methods via `RealTailscaleBackend`.
**Rationale**: `tsnet` is the same library phonesim uses, so the pattern is proven. Separate `pkg/tsbridge/` package keeps Tailscale lifecycle management out of `pkg/phoneserver/` (which is the gRPC server). Note: the spec originally said "libtailscale" but `tsnet` is the correct Go library — `libtailscale` is the C/mobile binding which is what gomobile produces from `tsnet`.
**Rejected**: Pure Kotlin Tailscale SDK (doesn't exist), VPN service approach (more complex, requires VPN permission), keeping the stub (doesn't satisfy spec), putting it in `pkg/phoneserver/` (wrong separation of concerns).

### Nix device loading approach (T044)
**Decision**: Add `Devices map[string]DeviceConfig` to Config struct. In `runDaemon()`, convert config devices to registry format and pass to `Merge()`.
**Rationale**: Minimal change — the NixOS module already writes the correct JSON, the merge function already works, only the reading side is broken. DeviceConfig struct maps 1:1 to the Nix module's device submodule fields.
**Rejected**: Loading devices from a separate Nix-specific file (unnecessary — config.json already has them), environment variable approach (too complex for structured data).

### Devices STATUS column approach (T054)
**Decision**: Add STATUS column that performs a concurrent Ping RPC to each device with a 2s timeout. Show "online"/"offline"/"unknown". Keep SOURCE column too.
**Rationale**: The `nix-key test` command already does Ping RPC with mTLS — reuse the same connectivity check logic. Concurrent checks prevent one slow device from blocking the whole table. 2s timeout is reasonable for LAN/Tailscale latency.
**Rejected**: Cached status from daemon (adds complexity to daemon state), remove SOURCE column (loses useful information), async loading with spinner (over-engineered for a CLI table).

### Verify-release-pipeline.sh fix approach (T072)
**Decision**: Update grep patterns to match actual YAML content. Section 4 should check for `workflow_run:` instead of `push:` and `pull_request:`.
**Rationale**: The simplest fix — change the grep patterns to match reality. No architectural change needed.

### Android loading state approach (T089)
**Decision**: Add `isLoading: Boolean` to each ViewModel's UI state. Set true during initial data fetch, false when data arrives or error occurs. Show `CircularProgressIndicator` when loading.
**Rationale**: Matches the pattern already used by PairingScreen and TailscaleAuthScreen. Consistent UX. Minimal code change — just add a state field and a conditional in the Composable.
**Rejected**: Shimmer/skeleton loading (over-engineered for this app), pull-to-refresh (different pattern, doesn't solve initial load).
