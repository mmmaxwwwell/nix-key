# Phase phase11-opentelemetry-distri — Review #1: REVIEW-CLEAN

**Date**: 2026-03-29T16:50:00Z
**Assessment**: Code is clean. No bugs, security issues, or correctness problems found.

**Scope**: 37 files changed, +4739/-107 lines | **Base**: 75dad182fc1790e5ea7eea31b1d2c796c189bae9~1
**Commits**: T049-T053 (OTEL host/phone tracing, Jaeger NixOS option, QR OTEL propagation, distributed trace E2E test) plus CLI commands (devices, revoke, status, export, config, logs, test)

**Review checklist**:
- Correctness: All span lifecycle management is correct (spans started and ended on all paths, including error paths). Parent-child relationships are properly maintained through context propagation.
- Security: No secrets exposed in spans (only fingerprints, device names, IPs). OTLP exporter uses insecure transport which is appropriate for localhost/Tailnet communication. mTLS cert file deletion on revoke is correctly implemented.
- Performance: No-op tracer when OTEL is disabled (zero overhead). Span creation is lightweight.
- Error handling: All error paths properly end spans and record errors. Cert warning collection silently ignores file read errors (appropriate for best-effort warnings).
- Resource management: TracerProvider shutdown is called in all teardown paths (phonesim, bridge, tests).

**Deferred** (optional improvements, not bugs):
- `internal/tracing/tracing.go` sets the global OTEL provider (`otel.SetTracerProvider`), which could theoretically conflict if multiple providers are initialized in the same process. Not a real issue since the daemon only creates one provider.
- The `config.go` iteration over `map[string]json.RawMessage` does not guarantee field order (Go map iteration is random). This is cosmetic — config output order may vary between runs.
- Pre-existing: 54 golangci-lint errcheck/staticcheck issues exist across the codebase (none introduced by this phase).
