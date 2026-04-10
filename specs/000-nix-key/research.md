# nix-key Architecture Decisions

All decisions confirmed by user unless noted otherwise.

---

## 1. Host Language: Go

**Decision:** Go for the host daemon and CLI.

**Rationale:** The `golang.org/x/crypto/ssh/agent` library provides a mature, well-tested SSH agent protocol implementation. Go compiles to a single static binary with no runtime dependencies, which makes Nix packaging trivial (no dependency graphs to manage, no language-specific build infrastructure). The Go ecosystem has strong library support for every component the host needs: gRPC, protobuf, age encryption, TLS, and structured logging.

**Rejected alternatives:**

- **Rust** -- Longer build times would slow CI feedback loops. The SSH agent protocol ecosystem in Rust is less mature than Go's `x/crypto/ssh/agent`. Cross-compilation for NixOS targets adds friction.
- **Python** -- Cannot produce a single static binary. Would introduce a runtime dependency on the Python interpreter, complicating NixOS packaging and systemd unit configuration.

---

## 2. Android Language: Kotlin

**Decision:** Kotlin with Jetpack Compose for the Android app.

**Rationale:** Kotlin is the natural choice for Android development and is Google's recommended language. Jetpack Compose provides a modern declarative UI framework. The `BiometricPrompt` API integrates cleanly with Kotlin coroutines for the user-confirmation flow that gates every signing request.

**Rejected alternatives:**

- **Flutter** -- Adds cross-platform complexity (Dart runtime, platform channels) for an app that only targets Android. No benefit since there is no iOS version planned.
- **Java** -- Legacy language for Android. More verbose, no coroutine support, Jetpack Compose is Kotlin-first.

---

## 3. Wire Protocol: gRPC with Protobuf

**Decision:** gRPC with protobuf for phone-to-host communication over mTLS.

**Rationale:** gRPC metadata enables automatic OTEL traceparent propagation without any custom plumbing -- a trace started on the host carries through to the phone and back. Protobuf provides type-safe contracts with generated code for both Go and Kotlin. Both languages have excellent gRPC support (grpc-go, grpc-kotlin). The 2-3MB APK overhead from gRPC/protobuf libraries is acceptable for this type of application.

**Rejected alternatives:**

- **JSON over raw TLS** -- Would require manual OTEL context propagation (custom headers, manual parsing). No type safety means protocol drift between host and phone implementations.
- **HTTP/2 with JSON** -- A middle ground that doesn't excel at anything. Still requires manual OTEL propagation. Still lacks type safety. Gains nothing over gRPC while losing its benefits.

---

## 4. Control Socket: Line-Delimited JSON over Unix Socket

**Decision:** Line-delimited JSON over a Unix domain socket for CLI-to-daemon IPC.

**Rationale:** Simple local IPC that requires roughly 20 lines of Go to implement. Each command is a single JSON line, each response is a single JSON line. Easy to debug with `socat`. Unix socket provides filesystem-based access control.

**Rejected alternatives:**

- **gRPC** -- Overkill for local IPC between components of the same binary. Adds protobuf compilation step and generated code for something that only needs a handful of simple commands.
- **HTTP** -- Unnecessary overhead (HTTP parsing, content-length handling, status codes) for a local socket that only the CLI uses.

---

## 5. Single Binary with Subcommands

**Decision:** One binary (`nix-key`) with subcommands: `nix-key daemon` for systemd, `nix-key pair`, `nix-key devices`, etc. for CLI operations.

**Rationale:** One Nix derivation to build, one binary to package. The systemd unit is a simple `ExecStart=nix-key daemon`. CLI commands connect to the running daemon over the control socket. No version-skew risk between separate daemon and CLI packages.

**Rejected alternatives:**

- **Separate daemon + CLI binaries** -- Two derivations, two packages, two version numbers to keep in sync. Added packaging complexity for zero functional benefit since both share the same Go module.

---

## 6. Age Encryption for Host Secrets

**Decision:** Age encryption (filippo.io/age) for host-side secret storage.

**Rationale:** Age is a simple, single-purpose encryption tool with a clean Go library. Machine-specific identity file stored at `~/.local/state/nix-key/age-identity.txt`. Encrypts paired device certificates and any other persistent secrets. No keyring, no key servers, no trust model to configure.

**Rejected alternatives:**

- **sops** -- Designed for managing secrets across teams and repositories. Overkill for a single-user, single-machine use case. Adds unnecessary complexity (key groups, creation rules, multiple backend support).
- **GPG** -- Complex keyring management, web-of-trust model, agent forwarding concerns. Would be ironic to use GPG agent infrastructure to bootstrap an SSH agent replacement.

---

## 7. Android Keystore Strategy

**Decision:** Dual strategy based on key type. ECDSA-P256 uses native Android Keystore hardware backing. Ed25519 uses BouncyCastle software implementation with a Keystore-backed AES wrapping key protecting the private key at rest.

**Rationale:** Android Keystore supports ECDSA-P256 in hardware (TEE/StrongBox) but does not support Ed25519 natively. Since Ed25519 is the modern SSH default and users will expect it, software implementation is necessary. Wrapping the Ed25519 private key with a hardware-backed AES key provides defense-in-depth: extracting the Ed25519 key requires compromising the Keystore's AES key.

**Rejected alternatives:**

- **Ed25519-only** -- No hardware backing available on Android. Would mean all keys are software-only, losing the security benefit of TEE/StrongBox.
- **ECDSA-only** -- Would force users to use a non-default SSH key type. Ed25519 is the recommended default in modern SSH configurations.

---

## 8. libtailscale for Android

**Decision:** Use libtailscale (official userspace Tailscale library) for Android networking.

**Rationale:** This is the same library the official Tailscale Android app uses. Provides userspace WireGuard networking via gomobile bindings. The phone joins the tailnet without requiring system-level VPN permission or a separate Tailscale app installation.

**Rejected alternatives:**

- **Custom WireGuard implementation** -- Reinventing the wheel. Would need to handle key exchange, DERP relay fallback, NAT traversal, and control plane integration. Massive effort for no benefit.
- **System-level Tailscale** -- Requires the VPN permission slot (Android allows only one VPN at a time). Would conflict with users who already run Tailscale or another VPN. Requires the Tailscale app to be always-on.

---

## 9. Headscale for Integration Tests

**Decision:** Headscale (self-hosted Tailscale control server) for end-to-end integration tests.

**Rationale:** NixOS provides `services.headscale` as a first-class module. Tests spin up a full Tailscale control plane, register test nodes, and exercise real WireGuard networking. Fully self-contained, fully reproducible, no external accounts or API keys needed.

**Rejected alternatives:**

- **Mock Tailscale** -- Would miss real networking bugs (NAT traversal, DERP relay, DNS). The whole point of E2E tests is to exercise the real network path.
- **Real Tailscale accounts** -- External dependency. Tests would fail if Tailscale's control plane is down. Not reproducible across environments. Would require managing test API keys.

---

## 10. Shared Go Library (pkg/phoneserver)

**Decision:** A shared Go package (`pkg/phoneserver`) containing the phone-side gRPC server logic, used by both the Android app (via gomobile) and the Go test simulator.

**Rationale:** The phone-side gRPC server handles signing requests, key listing, and confirmation flow. Sharing this logic between Android and the test simulator ensures protocol correctness. Interfaces for key storage (`KeyStore`) and user confirmation (`Confirmer`) allow platform-specific implementations (Android Keystore vs. in-memory test store, BiometricPrompt vs. auto-confirm).

**Rejected alternatives:**

- **Duplicate protocol logic in Kotlin** -- The Kotlin Android app and Go test simulator would inevitably diverge, producing bugs that only manifest in production but not in tests. Defeats the purpose of integration testing.
- **Kotlin-only server** -- Cannot be shared with the Go test simulator. Would require maintaining two separate gRPC server implementations.

---

## 11. QR Code: Base64 JSON with Full Certificate

**Decision:** QR code contains Base64-encoded JSON payload including the full phone client certificate. Approximately 1.2KB, fits within QR version 25.

**Rationale:** Including the full certificate enables immediate pinning on the host side. No trust-on-first-use (TOFU) vulnerability window. The host knows the exact certificate it will accept from the phone before the first real connection. QR version 25 handles the payload size comfortably.

**Rejected alternatives:**

- **Fingerprint-only** -- Creates a TOFU vulnerability. The host would need to accept any certificate matching the fingerprint on first connection, which could be intercepted.
- **Binary encoding** -- Not debuggable. When pairing fails, being able to decode the QR payload manually (base64 + JSON) is valuable for troubleshooting.
- **Multi-QR (animated/sequential)** -- Unnecessary complexity. The payload fits in a single QR code. Multi-QR adds failure modes (missed frames, ordering) for no benefit.

---

## 12. Pairing Protocol

**Decision:** Phone POSTs to the host's temporary HTTPS server during pairing. Host holds the connection open until the user confirms via CLI. Response includes the host's client certificate. Both certificates are exchanged over this secure connection.

**Rationale:** Physical proximity during pairing (user is scanning a QR code displayed on the host's screen) makes a simple protocol acceptable. The temporary HTTPS server only lives for the duration of the pairing flow. No persistent attack surface.

**Rejected alternatives:**

- **Diffie-Hellman key exchange** -- More complex protocol with more states and failure modes. The physical proximity requirement (QR scanning) already establishes a secure channel, making DH unnecessary.

---

## 13. CI/CD: Every Push to Main = Release

**Decision:** Every push to the `main` branch triggers the full CI pipeline and produces a release. Pipeline stages: lint, test, security scan, E2E tests, release, SBOM generation.

**Rationale:** Eliminates release friction. If code reaches `main`, it has passed through the `develop` integration branch and is ready for release. Automated releases ensure every deployed version has a corresponding tagged artifact and SBOM.

**Rejected alternatives:**

- **Manual releases** -- Friction leads to batched releases, which leads to larger changes per release, which increases risk. Antithetical to continuous delivery.
- **Direct to main** -- No integration staging. Broken code would immediately produce a broken release.

---

## 14. Branching: Main / Develop / Feature

**Decision:** Three-tier branching model. `main` is the release branch. `develop` is the integration branch. Feature branches are created off `develop`.

**Rationale:** For a security-critical project, the integration branch (`develop`) provides a staging area where features can be tested together before reaching `main`. Every push to `main` triggers a release, so `main` must always be in a releasable state.

**Rejected alternatives:**

- **Trunk-based development** -- Risky for a security-critical project. A bad merge directly to the release branch would immediately produce a broken release.
- **GitFlow** -- Full GitFlow with release branches, hotfix branches, and support branches is overkill for a project with a single supported version.

---

## 15. Structured JSON Logging: Go slog (Host), Timber (Android)

**Decision:** Go's standard `log/slog` package for host-side structured logging. Timber with a custom JSON formatter for Android-side structured logging.

**Rationale:** `slog` is the standard structured logging package in Go (since 1.21), well-supported across the ecosystem. Timber is the standard Android logging library, and a custom `Tree` implementation can format logs as JSON for consistency with the host side. Both produce machine-parseable structured logs suitable for OTEL integration.

**Rejected alternatives:**

- **zerolog** -- Less ecosystem support than `slog` for new Go projects. Since `slog` became standard, third-party loggers have less justification.
- **logrus** -- In maintenance mode. The maintainer recommends migrating to other solutions.

---

## 16. Error Hierarchy: NixKeyError Base

**Decision:** Typed error hierarchy rooted at `NixKeyError` with subtypes: `ConnectionError`, `TimeoutError`, `CertError`, `ConfigError`, `ProtocolError`. All errors are mapped to `SSH_AGENT_FAILURE` when returned through the SSH agent protocol.

**Rationale:** Typed errors provide structured context for debugging and logging. The SSH agent protocol only supports a single failure code (`SSH_AGENT_FAILURE`), so internal error types are for diagnostics only -- they appear in logs and traces but not in the SSH protocol response.

**Rejected alternatives:**

- **Flat error codes** -- Less context for debugging. An integer error code in a log line tells you what failed but not why.
- **External error library** (e.g., pkg/errors, cockroachdb/errors) -- Unnecessary dependency. Go 1.13+ error wrapping with `fmt.Errorf` and `%w` covers the needed functionality.

---

## 17. Config Format: JSON

**Decision:** JSON for all configuration files, written by the NixOS module and read by the daemon, CLI, and tests.

**Rationale:** The NixOS module generates configuration as a Nix attribute set, which serializes naturally to JSON via `builtins.toJSON`. The daemon and CLI read the config without needing to evaluate Nix. Tests can generate config programmatically. Three configuration layers with increasing precedence: NixOS module (generates the file), config file (read at startup), environment variables (override at runtime).

**Rejected alternatives:**

- **TOML** -- Less native Nix tooling. Nix has `builtins.toJSON` but no `builtins.toTOML`. Would require additional serialization logic in the NixOS module.
- **YAML** -- Parsing ambiguities (the Norway problem, implicit type coercion). More complex parser for no benefit over JSON.

---

## 18. NixOS User Service (systemd user)

**Decision:** Run the daemon as a systemd user service, analogous to `gpg-agent`.

**Rationale:** The SSH agent is a per-user concern. A user service runs with the user's permissions, has access to the user's state directory, and can set `SSH_AUTH_SOCK` via `environment.d`. This matches the established pattern for SSH and GPG agents on Linux.

**Rejected alternatives:**

- **System service** -- Wrong scope. Would run as root or a dedicated service user, requiring privilege separation to access per-user state. A system service managing per-user SSH keys is an anti-pattern.
- **Socket activation** -- Adds complexity for a daemon that should be always-running. The daemon needs to maintain persistent connections to paired phones and respond to SSH agent requests with minimal latency. Cold-start on first SSH use would add noticeable delay.
