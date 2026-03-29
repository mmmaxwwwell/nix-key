# nix-key Constitution

## Core Principles

### I. Nix-First
Every component on the host side is a Nix derivation or NixOS module. The project provides a flake with a NixOS module (`services.nix-key`), a CLI package, and integration tests runnable via `nix flake check`. No Docker, no global installs. The Android app is the only non-Nix artifact.

### II. Security by Default (NON-NEGOTIABLE)
All communication uses mutual TLS with certificate pinning. SSH private keys never leave the Android device. Certificates are self-signed and exchanged out-of-band via QR code during onboarding. No plaintext secrets on disk — all sensitive material (mTLS private keys, pairing tokens, etc.) MUST be encrypted at rest. Use Android Keystore on mobile, age or sops encryption on host. File permissions (0600) are defense-in-depth, not the primary protection. Every security decision requires explicit rationale.

### III. Test-First (NON-NEGOTIABLE)
TDD mandatory: tests written → tests fail → implement → tests pass. Integration tests cover the full flow: NixOS VM tests for the service module, instrumented tests for the Android app, end-to-end tests for the mTLS handshake and SSH agent protocol. No feature ships without a corresponding test.

### IV. Unix Philosophy
The host daemon is a single-purpose systemd user service. Configuration lives in `~/.config/nix-key/`. Runtime state in `~/.local/state/nix-key/`. The CLI is a standalone tool that communicates with the daemon. SSH agent protocol compliance means standard `SSH_AUTH_SOCK` integration.

### V. Minimal Trust Surface
The phone only connects to Tailnet when the app is actively open. The server can be configured to not enumerate all keys. Each key has independent confirmation policy (biometric, password, auto-approve, deny). Device authorization requires mutual confirmation (CLI + phone).

### VI. Simplicity
Start with the minimum viable protocol. No over-abstraction. One way to do each thing. Configuration is a single Nix attrset that generates a JSON config file. The Android app has one screen per concern.

## Security Requirements

- mTLS with self-signed certificates and key pinning
- SSH private keys generated and stored in Android Keystore
- Biometric/password confirmation configurable per key
- Device onboarding requires physical proximity (QR code)
- Mutual authorization: both CLI and phone must confirm pairing
- Config file contains no secrets — secrets reference Keystore/encrypted files
- Tailscale connection only active when app is foregrounded

## Observability

- Structured JSON logging with correlation IDs on both host and Android
- OpenTelemetry distributed tracing across the mTLS boundary (host ↔ Android)
- Trace context propagated in request headers
- Exportable to Jaeger/OTLP collector (configurable endpoint in NixOS module)
- No metrics pipeline or dashboards (can be added later)

## Development Workflow

- All host code is Nix-built and tested via `nix flake check`
- Android app uses Gradle with standard Android testing
- Integration tests use NixOS VM test framework
- Every PR must pass: unit tests, integration tests, NixOS module tests
- Security-relevant changes require review of threat model impact

## Governance

Constitution supersedes all other practices. Amendments require documentation and rationale. Security principles (II, V) are non-negotiable — any relaxation requires explicit threat model justification.

**Version**: 1.0.0 | **Ratified**: 2026-03-28 | **Last Amended**: 2026-03-28
