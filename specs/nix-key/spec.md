# Feature Specification: nix-key

**Created**: 2026-03-28
**Status**: Draft
**Preset**: public
**Input**: Android SSH key manager that acts like a YubiKey, communicating with a NixOS host over Tailscale with mTLS. Full CLI, NixOS module, integration tests, distributed tracing.

---

## User Scenarios & Testing

### User Story 1 — SSH Signing via Phone (Priority: P1)

A developer runs `git commit` or `ssh user@host`. The SSH client asks the nix-key agent for a signature. The host daemon connects to the phone over Tailscale mTLS. The phone displays a confirmation prompt showing the host name and key being used. The user approves (with biometric/password per key policy). The phone signs the data in Android Keystore and returns the signature. The SSH operation completes.

**Why this priority**: This is the core value proposition — the phone replaces a YubiKey for SSH signing.

**Independent Test**: NixOS VM test with a simulated phone TLS server. Host daemon receives an SSH agent sign request, forwards to the simulated phone, gets a signature, returns it to the SSH client. Verify `ssh-add -L` lists keys and `ssh -T git@github.com`-style operations succeed.

**Acceptance Scenarios**:

1. **Given** the phone app is open and connected to Tailnet, **When** an SSH sign request arrives at the host daemon, **Then** the phone shows a confirmation prompt within 2 seconds.
2. **Given** the user approves the sign request, **Then** the signature is returned to the SSH client and the operation succeeds.
3. **Given** the user denies the sign request, **Then** the SSH client receives SSH_AGENT_FAILURE.
4. **Given** the phone app is closed (not on Tailnet), **When** a sign request arrives, **Then** the daemon returns SSH_AGENT_FAILURE after `connectionTimeout` seconds.
5. **Given** a key is configured with biometric confirmation, **When** a sign request arrives, **Then** the phone shows BiometricPrompt before signing.
6. **Given** a key is configured with auto-approve, **When** a sign request arrives, **Then** the phone signs immediately without user interaction and returns the signature.

---

### User Story 2 — Device Onboarding / Pairing (Priority: P1)

A user runs `nix-key pair` on their NixOS machine. The CLI spins up a temporary HTTPS server bound to the Tailscale interface and displays a QR code in the terminal. The QR code contains the host's Tailscale IP, temporary pairing port, TLS certificate fingerprint, one-time pairing token, and optionally the OTEL collector endpoint. The user opens the nix-key Android app, scans the QR code, and the app asks "Connect to `hostname`? [Accept/Deny]". If the host has OTEL configured, the app also prompts "Enable tracing? Traces will be sent to `100.x.x.x:4317` [Accept/Deny]". On accept, the phone connects to the temporary HTTPS endpoint and presents its own TLS certificate, Tailscale IP, and listening port. The host CLI shows "Phone `Pixel 8` wants to pair. Authorize? [y/N]". On mutual confirmation, both sides store each other's certificates and connection info. The temporary HTTPS server shuts down.

**Why this priority**: Pairing is required before any signing can happen. Co-P1 with Story 1.

**Independent Test**: Integration test that runs the pairing flow with a simulated phone client. Verify certificates are exchanged, device appears in `nix-key devices`, and the phone's stored host info is correct.

**Acceptance Scenarios**:

1. **Given** `nix-key pair` is running, **When** the phone scans the QR code, **Then** the app shows the host name and asks for confirmation.
2. **Given** both sides confirm, **Then** the phone appears in `nix-key devices` with correct cert fingerprint and Tailscale IP.
3. **Given** the user denies on the phone, **Then** pairing is aborted, no state is saved on either side.
4. **Given** the user denies on the host CLI, **Then** pairing is aborted, no state is saved on either side.
5. **Given** the one-time pairing token is reused, **Then** the connection is rejected.
6. **Given** the host has OTEL configured, **When** the phone scans the QR code, **Then** the app prompts to accept OTEL config in addition to the pairing confirmation.
7. **Given** the phone accepts OTEL config, **Then** the phone stores the collector endpoint and enables tracing.
8. **Given** the phone denies OTEL config, **Then** pairing still succeeds but tracing is disabled on the phone.

---

### User Story 3 — SSH Key Creation on Phone (Priority: P1)

A user opens the nix-key app and creates a new SSH key. They choose a name, key type (Ed25519 default, ECDSA-P256), and confirmation policy (always ask, biometric, password, biometric+password, auto-approve). The key is generated inside Android Keystore (hardware-backed when available). The public key is displayed and can be exported via clipboard, share sheet, or QR code.

**Why this priority**: Keys must exist before signing can work. Co-P1 with Stories 1 and 2.

**Independent Test**: Android instrumented test that creates a key, verifies it exists in Keystore, exports the public key, and verifies the exported format is valid SSH public key format.

**Acceptance Scenarios**:

1. **Given** the user taps "Create Key", **When** they fill in name and select Ed25519, **Then** a key is generated in Android Keystore.
2. **Given** a key exists, **When** the user taps "Export Public Key", **Then** the public key is available in standard `ssh-ed25519 AAAA... name` format.
3. **Given** a key exists, **When** the user changes its confirmation policy to "biometric", **Then** subsequent sign requests for that key require BiometricPrompt.
4. **Given** a key has auto-approve selected, **Then** the app shows a warning about the security implications before saving.
5. **Given** the user creates a key, **Then** the private key material is never extractable from Android Keystore.

---

### User Story 4 — Key Listing (Priority: P2)

When the host daemon receives an SSH agent "list keys" request, it connects to all reachable paired phones and collects their public keys. The combined list is returned to the SSH client. If `allowKeyListing` is disabled on the host, an empty list is returned. If the phone has key listing disabled in its settings, that phone returns an empty list (even if the host allows listing).

**Why this priority**: Enables standard SSH key selection. Not strictly required (host could send blind sign requests) but expected by most SSH workflows.

**Independent Test**: NixOS VM test with simulated phones. Verify `ssh-add -L` returns the combined key list. Verify empty list when either side disables listing.

**Acceptance Scenarios**:

1. **Given** two paired phones with 3 keys total, **When** the host receives a list-keys request, **Then** all 3 public keys are returned.
2. **Given** `allowKeyListing = false` on host, **When** a list-keys request arrives, **Then** an empty list is returned without contacting phones.
3. **Given** a phone has "deny key listing" enabled, **When** the host requests keys from that phone, **Then** that phone returns an empty list.
4. **Given** one phone is unreachable, **When** a list-keys request arrives, **Then** keys from reachable phones are returned (unreachable phone is skipped after `connectionTimeout`).

---

### User Story 5 — CLI Device Management (Priority: P2)

The user manages authorized phones via the `nix-key` CLI:
- `nix-key devices` — lists all authorized phones with name, Tailscale IP, cert fingerprint, last seen time, and connection status
- `nix-key revoke <device>` — revokes a phone's authorization, deletes its cert, removes from devices list
- `nix-key status` — shows daemon status, connected devices, available keys, socket path
- `nix-key export <key-id>` — prints a public key to stdout in SSH format
- `nix-key config` — shows current configuration (from NixOS module or config file)
- `nix-key logs` — tails daemon logs (structured JSON, human-readable formatting)
- `nix-key test <device>` — verifies the connection to a specific phone is working (mTLS handshake + ping)

**Why this priority**: Essential for day-to-day management but pairing and signing work without it.

**Independent Test**: Integration test that runs each CLI command against a running daemon with test fixtures. Verify output format and state changes.

**Acceptance Scenarios**:

1. **Given** a paired device, **When** `nix-key devices` is run, **Then** the device is listed with correct info.
2. **Given** a paired device, **When** `nix-key revoke pixel-8` is run, **Then** the device is removed from the devices list and its cert is deleted.
3. **Given** a running daemon, **When** `nix-key status` is run, **Then** daemon status, connected devices, and available keys are shown.
4. **Given** a reachable phone, **When** `nix-key test pixel-8` is run, **Then** the output confirms mTLS handshake success and round-trip time.
5. **Given** an unreachable phone, **When** `nix-key test pixel-8` is run, **Then** the output shows connection failure with a clear error message.

---

### User Story 6 — NixOS Module Configuration (Priority: P2)

A NixOS user adds `nix-key` to their flake inputs and configures `services.nix-key` in their system configuration. The module:
- Installs the `nix-key` CLI binary
- Creates a systemd user service `nix-key-agent.service`
- Sets `SSH_AUTH_SOCK` via `systemd.user.services` environment
- Writes all settings to `~/.config/nix-key/config.json`
- Manages certificate directory permissions
- Merges Nix-declared devices with runtime-paired devices
- Optionally runs a Jaeger instance for tracing

**Why this priority**: The primary distribution mechanism. Critical for the "super easy to import" goal.

**Independent Test**: NixOS VM test that evaluates the module with a test config, verifies the service starts, config file is written correctly, and SSH_AUTH_SOCK is set.

**Acceptance Scenarios**:

1. **Given** `services.nix-key.enable = true`, **When** the system is built, **Then** the `nix-key-agent` systemd user service is created and enabled.
2. **Given** a config with `port = 29418`, **When** the service starts, **Then** `~/.config/nix-key/config.json` contains `"port": 29418`.
3. **Given** devices declared in Nix config, **When** the daemon starts, **Then** those devices are available alongside runtime-paired devices.
4. **Given** `services.nix-key.tracing.jaeger.enable = true`, **When** the system is built, **Then** a Jaeger service is running and the nix-key daemon is configured to export traces to it.
5. **Given** a flake user, **When** they add `nix-key.nixosModules.default` to their modules, **Then** `services.nix-key` is available.
6. **Given** a non-flake user, **When** they import via `fetchTarball`, **Then** `services.nix-key` is available.

---

### User Story 7 — Distributed Tracing (Priority: P3)

When OTEL is configured on the host and the phone has accepted tracing during pairing, every signing request generates a distributed trace visible in Jaeger. The trace shows the full request lifecycle across both systems: SSH agent request → device lookup → mTLS connection → phone prompt → user response → signing → response.

**Why this priority**: Debugging and observability, not core functionality.

**Independent Test**: Integration test that performs a signing request with OTEL configured, collects the trace, and verifies span hierarchy and trace context propagation across the mTLS boundary.

**Acceptance Scenarios**:

1. **Given** OTEL is configured on host and phone, **When** a sign request completes, **Then** a trace with spans from both host and phone is visible in the collector.
2. **Given** OTEL is disabled, **When** a sign request completes, **Then** no trace overhead is added.
3. **Given** a trace is collected, **Then** the W3C `traceparent` header is present in the mTLS request and the phone's spans are children of the host's spans.

---

### Edge Cases & Failure Modes

- **FR-E01**: When the phone is unreachable (app closed, no Tailnet), the daemon MUST return SSH_AGENT_FAILURE after `connectionTimeout` seconds with no retry.
- **FR-E02**: When the user denies a sign request, the daemon MUST return SSH_AGENT_FAILURE immediately.
- **FR-E03**: When the user does not respond within `signTimeout`, the daemon MUST return SSH_AGENT_FAILURE.
- **FR-E04**: When multiple concurrent sign requests arrive, the phone MUST queue them and show each as a separate confirmation prompt.
- **FR-E05**: When the mTLS handshake fails (expired/revoked cert, wrong fingerprint), the daemon MUST log the attempt with details and return failure.
- **FR-E06**: When the config file is missing or corrupted, the daemon MUST refuse to start with a clear error message in systemd journal.
- **FR-E07**: When the daemon restarts mid-sign, pending requests are lost. SSH clients handle this naturally with retry.
- **FR-E08**: When the phone loses Tailscale connection mid-sign, the mTLS connection drops and the daemon returns failure after detecting the disconnect.
- **FR-E09**: When a previously revoked device attempts to connect during normal operation, the host MUST reject the mTLS handshake (cert fingerprint is no longer in the authorized devices list).
- **FR-E10**: When the one-time pairing token is replayed, the server MUST reject the connection.
- **FR-E11**: When `nix-key pair` is run but the Tailscale interface is not available, the CLI MUST fail with a clear error.
- **FR-E12**: When a phone's Tailscale IP changes (e.g., after Tailscale re-auth), the next successful connection MUST update the stored IP. The cert fingerprint is the identity, not the IP.

---

## Requirements

### Functional Requirements — SSH Agent

- **FR-001**: The host daemon MUST implement the SSH agent protocol (RFC draft) for `SSH2_AGENTC_REQUEST_IDENTITIES` (list keys) and `SSH2_AGENTC_SIGN_REQUEST` (sign).
- **FR-002**: The daemon MUST listen on a Unix socket at the configured `socketPath` (default `$XDG_RUNTIME_DIR/nix-key/agent.sock`).
- **FR-003**: The daemon MUST set `SSH_AUTH_SOCK` to the socket path via `~/.config/environment.d/50-nix-key.conf` (see FR-069a for NixOS module implementation).
- **FR-004**: Sign requests MUST be forwarded to the phone that holds the requested key, identified by public key fingerprint.
- **FR-005**: If `allowKeyListing` is `false`, the daemon MUST return an empty key list without contacting phones. See also FR-054 (phone-side denial) and FR-066 (documentation note).
- **FR-006**: The daemon MUST support multiple paired phones, each with independent keys.
- **FR-007**: List-keys requests MUST skip unreachable phones after `connectionTimeout` and return keys from reachable phones only.

### Functional Requirements — mTLS & Networking

- **FR-010**: All host↔phone communication MUST use mutual TLS with self-signed certificates.
- **FR-011**: Both sides MUST pin the peer's certificate by fingerprint (no CA trust chain).
- **FR-012**: During normal operation, the host daemon MUST connect to phones as a TLS client; phones MUST run a TLS server when the app is active. During pairing, the direction is reversed (phone connects to host's temporary HTTPS server).
- **FR-013**: The phone MUST use a userspace Tailscale implementation via `libtailscale` (connected to Tailnet only when the app is foregrounded).
- **FR-013a**: The Android app MUST support Tailscale authentication on first launch. The user provides a Tailscale auth key or completes an OAuth flow to join the Tailnet. The auth state MUST be persisted in encrypted storage so re-authentication is not required on every app open.
- **FR-013b**: The NixOS module MUST support `services.nix-key.tailscale.authKeyFile` for providing a pre-authorized Tailscale auth key (useful for automated setups and testing).
- **FR-014**: The phone's TLS server MUST bind only to the Tailscale interface.
- **FR-015**: During normal operation, the host daemon MUST only initiate mTLS connections to phones via the configured Tailscale interface.
- **FR-016**: Certificate exchange MUST happen during the pairing flow: the QR code bootstraps the initial TLS connection, and certificates are exchanged over that connection (out-of-band relative to the Tailnet).

### Functional Requirements — Wire Protocol

- **FR-017**: The host↔phone protocol MUST use gRPC with protobuf over mTLS, with the following service definition:
  - `ListKeys` — request: empty; response: repeated key entries (public key blob, key type, display name, fingerprint)
  - `Sign` — request: key fingerprint, data to sign, trace context; response: signature bytes or error
  - `Ping` — request: empty; response: timestamp (for `nix-key test`)
- **FR-018**: The gRPC service MUST propagate W3C traceparent via gRPC metadata for OpenTelemetry integration.
- **FR-019**: The protobuf schema MUST be versioned and included in the repository for both Go and Kotlin code generation.

### Functional Requirements — Pairing

- **FR-020**: `nix-key pair` MUST spin up a temporary HTTPS server bound to the Tailscale interface.
- **FR-021**: The QR code MUST contain: host Tailscale IP, temporary pairing port, host's self-signed TLS server certificate (full, for pinning on first connect), one-time pairing token, and optionally OTEL collector endpoint.
- **FR-022**: The phone MUST prompt "Connect to `hostname`?" before connecting to the pairing endpoint.
- **FR-023**: If OTEL config is present in the QR code, the phone MUST prompt "Enable tracing?" separately from the pairing confirmation.
- **FR-024**: On connection, the phone MUST POST to the pairing endpoint with: `{phoneName, phoneTailscaleIp, phoneListenPort, phoneServerCert, oneTimeToken}`. The phone's server cert is the cert it will use as a TLS server during normal operation.
- **FR-025**: The host CLI MUST display the phone's name and Tailscale IP, then prompt "Phone `name` wants to pair. Authorize? [y/N]". The HTTP connection is held open until the host user responds.
- **FR-026**: On host confirmation, the server MUST respond with `{hostName, hostClientCert, status: "approved"}`. The host's client cert is the cert the host will present when connecting to the phone during normal operation. Both sides MUST store the peer's cert and connection info.
- **FR-027**: The one-time pairing token MUST be invalidated after use (success or failure).
- **FR-028**: The temporary HTTPS server MUST shut down after pairing completes or times out.
- **FR-029**: One host MUST support multiple paired phones.
- **FR-030**: One phone MUST support multiple paired hosts.
- **FR-032**: mTLS certificates MUST have a configurable expiry (default 1 year). The `nix-key status` command MUST warn when a device cert is within 30 days of expiry. Re-pairing is required to rotate certs.
- **FR-031**: `nix-key pair` MUST generate two certificate pairs: (a) a TLS server cert/key for the temporary pairing HTTPS server, and (b) a TLS client cert/key that the host will use during normal operation to connect to phones.

### Functional Requirements — Key Management (Android)

- **FR-040**: The app MUST generate SSH keys with private key material protected by Android Keystore. ECDSA-P256 keys MUST use native Keystore hardware backing (TEE/StrongBox) when available. Ed25519 keys MUST be software-generated (via BouncyCastle) and encrypted at rest with a Keystore-backed AES wrapping key, since Android Keystore does not natively support Ed25519.
- **FR-041**: Supported key types: Ed25519 (default, software + Keystore-wrapped), ECDSA-P256 (native Keystore hardware-backed).
- **FR-042**: Private key material MUST never be extractable from Android Keystore.
- **FR-043**: Public keys MUST be exportable in standard SSH format (`ssh-ed25519 AAAA... name`).
- **FR-044**: Export methods: clipboard, share sheet, QR code.
- **FR-045**: Each key MUST have a configurable confirmation policy: always ask (default), biometric only, password only, biometric+password, auto-approve.
- **FR-046**: Auto-approve MUST show a security warning before the user can enable it.
- **FR-047**: Key deletion MUST require biometric or password confirmation.
- **FR-048**: Each key MUST have a user-editable display name.

### Functional Requirements — Sign Request Handling (Android)

- **FR-050**: When a sign request arrives, the app MUST show an overlay/dialog with: host name, key name, data hash (truncated).
- **FR-051**: The confirmation prompt MUST trigger the key's configured confirmation policy (biometric/password/both/none).
- **FR-052**: [REMOVED — SSH agent protocol always includes a key blob in sign requests; this scenario is unreachable. When `allowKeyListing` is disabled, users must configure their SSH client with the correct public key via `IdentityFile`.]
- **FR-053**: Multiple concurrent sign requests MUST be queued and shown as separate prompts.
- **FR-054**: The phone MUST have a setting to deny key listing regardless of host configuration.

### Functional Requirements — NixOS Module

- **FR-060**: The module MUST be importable via `nix-key.nixosModules.default` (flakes) or `fetchTarball` (channels).
- **FR-061**: `services.nix-key.enable` MUST create a systemd user service `nix-key-agent.service`.
- **FR-062**: The module MUST write all configuration to `~/.config/nix-key/config.json`.
- **FR-063**: The module MUST support declarative device definitions in the `devices` attrset.
- **FR-064**: Declarative devices MUST be merged with runtime-paired devices from `~/.local/state/nix-key/devices.json`.
- **FR-065**: Each device in the `devices` attrset MUST support optional `clientCert` and `clientKey` paths for programmatic mTLS cert definition, with `null` meaning "set by pairing".
- **FR-066**: The `allowKeyListing` option MUST include documentation noting that the phone can also independently deny listing, resulting in an empty array.
- **FR-067**: The module MUST support `services.nix-key.tracing.otelEndpoint` for OTEL collector configuration.
- **FR-068**: The module MUST support `services.nix-key.tracing.jaeger.enable` to optionally run a local Jaeger instance.
- **FR-069**: The flake MUST export: `nixosModules.default`, `packages.default` (CLI), `checks` (all tests), `overlays.default`.
- **FR-069a**: The module MUST create `~/.config/environment.d/50-nix-key.conf` containing `SSH_AUTH_SOCK=<socketPath>` so all user login sessions pick up the agent socket path automatically.

### Functional Requirements — CLI

- **FR-070**: `nix-key pair` MUST display a QR code in the terminal and wait for phone connection.
- **FR-071**: `nix-key devices` MUST list all authorized phones with name, Tailscale IP, cert fingerprint, last seen, connection status.
- **FR-072**: `nix-key revoke <device>` MUST remove the device and delete its cert.
- **FR-073**: `nix-key status` MUST show daemon status, connected devices, available keys, socket path.
- **FR-074**: `nix-key export <key-id>` MUST print the public key in SSH format to stdout. `key-id` is the key's SHA256 fingerprint (as shown by `nix-key status`) or a unique prefix match.
- **FR-075**: `nix-key config` MUST show current configuration.
- **FR-076**: `nix-key logs` MUST tail daemon logs with human-readable formatting.
- **FR-077**: `nix-key test <device>` MUST verify mTLS handshake and round-trip to the specified phone.

### Functional Requirements — Distributed Tracing

- **FR-080**: The host daemon MUST support OpenTelemetry trace export via OTLP (gRPC or HTTP).
- **FR-081**: The Android app MUST support OpenTelemetry trace export via OTLP.
- **FR-082**: W3C `traceparent` header MUST be propagated in mTLS requests between host and phone.
- **FR-083**: Host spans: `ssh-sign-request`, `device-lookup`, `mtls-connect`, `return-signature`.
- **FR-084**: Phone spans: `handle-sign-request`, `show-prompt`, `user-response`, `keystore-sign`.
- **FR-085**: Phone spans MUST be children of the host's `mtls-connect` span (linked via traceparent).
- **FR-086**: Tracing MUST be disabled by default. No overhead when disabled.
- **FR-087**: During pairing, if the host has OTEL configured, the QR code MUST include the collector endpoint.
- **FR-088**: The phone MUST prompt the user to accept or deny OTEL config during pairing.

### Functional Requirements — Logging

- **FR-090**: The host daemon MUST use structured JSON logging with correlation IDs.
- **FR-091**: Log levels: DEBUG, INFO, WARN, ERROR, FATAL. Default: INFO.
- **FR-092**: Log level MUST be configurable via `services.nix-key.logLevel`.
- **FR-093**: The Android app MUST use structured logging correlated with trace IDs when tracing is enabled.
- **FR-094**: Security events (pairing attempts, sign requests, revocations, failed mTLS handshakes) MUST be logged at INFO or above.

### Functional Requirements — Error Handling

- **FR-095**: The host daemon MUST use a typed error hierarchy: `NixKeyError` → `ConnectionError`, `TimeoutError`, `CertError`, `ConfigError`, `ProtocolError`.
- **FR-096**: All errors MUST include an error code, human-readable message, and correlation ID.
- **FR-097**: Error messages to SSH clients MUST be sanitized (no internal paths, cert details, or stack traces).

### Functional Requirements — Configuration

- **FR-098**: All settings from the NixOS module MUST be written to `~/.config/nix-key/config.json`.
- **FR-099**: The config file MUST be readable by the CLI, daemon, and tests without requiring NixOS evaluation.
- **FR-100**: The daemon MUST fail-fast on startup if the config file is invalid (schema validation).

### Functional Requirements — Secrets at Rest

- **FR-101**: All mTLS private keys on the host MUST be encrypted at rest using age encryption. The age identity key MUST be derived from or protected by a passphrase or system-specific key.
- **FR-102**: The daemon MUST decrypt mTLS private keys into memory at startup and never write plaintext keys to disk.
- **FR-103**: The NixOS module MUST support `services.nix-key.secrets.ageKeyFile` to specify the age identity file for decrypting cert private keys.
- **FR-104**: `nix-key pair` MUST encrypt all generated private keys with age before writing to disk.

### Key Entities

- **Device**: A paired phone. Attributes: name, Tailscale IP, listening port, cert fingerprint, client cert path (optional), last seen timestamp, pairing source (nix-declared or runtime).
- **SSHKey**: An SSH key on a phone. Attributes: public key blob, key type, display name, fingerprint. Note: private key exists only in Android Keystore.
- **SignRequest**: A pending SSH signing request. Attributes: request ID, key fingerprint, data to sign, host name, timestamp, status (pending/approved/denied/timeout).
- **PairingSession**: A temporary pairing session. Attributes: one-time token, host server cert (for temp HTTPS), host client cert (for normal operation), temporary port, OTEL config (optional), status, expiry. Pairing protocol: phone POSTs `{phoneName, phoneTailscaleIp, phoneListenPort, phoneServerCert, oneTimeToken}`, host responds with `{hostName, hostClientCert, status}` after CLI user confirms.

---

## Enterprise Infrastructure

### Logging
- **Library**: Go `slog` (host), Android `timber` + custom JSON formatter (phone)
- **Format**: Structured JSON, one object per line
- **Levels**: DEBUG, INFO, WARN, ERROR, FATAL (configurable, default INFO)
- **Correlation IDs**: Generated at SSH agent request entry, propagated via trace context to phone
- **Destination**: systemd journal (host), logcat (phone)

### Error Handling
- **Hierarchy**: `NixKeyError` base → `ConnectionError`, `TimeoutError`, `CertError`, `ConfigError`, `ProtocolError`
- **Propagation**: Errors wrap with context at each layer, original error preserved
- **SSH client facing**: Sanitized — SSH_AGENT_FAILURE with no details (SSH protocol limitation). Details in daemon logs only.

### Configuration
- **Format**: JSON (`~/.config/nix-key/config.json`)
- **Layers**: NixOS module (generates file) → config file (read by daemon/CLI) → environment variables (override, for testing)
- **Validation**: JSON schema validation on daemon startup, fail-fast on invalid config
- **Secrets**: mTLS private keys stored encrypted at rest in `~/.local/state/nix-key/certs/` using age encryption with a Keystore-derived key (or sops). Decrypted into memory at daemon startup. Config references encrypted file paths, never plaintext keys. File permissions 0600 as defense-in-depth.

### Security
- **mTLS**: Self-signed certificates with fingerprint pinning, no CA chain
- **Key storage (Android)**: ECDSA-P256 in Android Keystore (hardware-backed). Ed25519 software-generated, encrypted with Keystore-backed AES wrapping key.
- **Key storage (host)**: mTLS private keys encrypted at rest with age. Decrypted into memory only.
- **Cert exchange**: Out-of-band via QR code (physical proximity required). Full cert in QR, not just fingerprint.
- **Wire protocol**: gRPC over mTLS with protobuf. Automatic OTEL trace propagation via gRPC metadata.
- **Input validation**: All protocol messages validated against protobuf schema at system boundary
- **Scanning**: Tier 1 + Tier 1.5 (Trivy, Semgrep, Gitleaks, Snyk, SonarCloud, OpenSSF Scorecard, `govulncheck`)
- **Pre-commit**: Gitleaks hook to prevent secret commits

### Observability
- **Tracing**: OpenTelemetry with OTLP export, W3C traceparent propagation across mTLS boundary
- **Collector**: Optional Jaeger via NixOS module (`services.nix-key.tracing.jaeger.enable`)
- **Metrics**: Not included (can be added later)
- **Error reporting**: Structured logs with correlation IDs (no Sentry — single user)

### CI/CD
- **Platform**: GitHub Actions with Nix
- **Cache**: Cachix (or similar)
- **Quality gates**: `nix flake check` (Go tests + NixOS VM tests), security scan, lint
- **Scanning**: Trivy, Semgrep, Gitleaks, govulncheck — runs on every push to main
- **Android**: Gradle build + instrumented tests (separate CI job)
- **SBOM**: Trivy CycloneDX on every release
- **Release model**: Every push to main is a release. Development happens on `develop` branch. PRs merge to `develop`, then `develop` merges to `main` triggers the full pipeline: lint → test → security → build → release (tag + artifacts + SBOM).
- **Branching**: `main` (release), `develop` (integration), feature branches off `develop`

### Graceful Shutdown
- **Timeout**: 30 seconds
- **Sequence**: Stop accepting new SSH agent connections → finish in-flight sign requests (with deadline) → close phone connections → close Unix socket → exit
- **Signal**: SIGTERM from systemd, SIGINT from terminal

### Health Checks
- CLI `nix-key status` serves as health check. No HTTP health endpoint (not a web service).
- `nix-key test <device>` verifies per-device connectivity.

### Rate Limiting
- Not applicable — single user, Unix socket interface. mTLS prevents unauthorized network access.

### Developer Experience
- **Task runner**: Makefile + `nix develop` shell
- **One-command dev**: `nix develop` provides Go, Android SDK (for local testing), test tools
- **Test commands**: `nix flake check` (all tests), `go test ./...` (unit/integration), `nix build .#checks.x86_64-linux.nixos-test` (VM tests)
- **Debugging**: VS Code `launch.json` for Go daemon, Android Studio for the app

### Branching
- `main` = release branch. Every push to main triggers full CI + release.
- `develop` = integration branch. PRs merge here first.
- Feature branches off `develop`. Spec-kit feature work uses `develop` as base.

---

## Testing

### NixOS VM Tests
- **T-NM-01**: Module evaluation — verify service definition is correct with various config options [FR-060 through FR-069]
- **T-NM-02**: Service lifecycle — verify daemon starts, creates socket, sets SSH_AUTH_SOCK [FR-001, FR-002, FR-003, FR-061]
- **T-NM-03**: Config file generation — verify config.json is written with all settings [FR-062, FR-098, FR-099]
- **T-NM-04**: Device merge — verify Nix-declared devices are merged with runtime-paired devices [FR-063, FR-064]
- **T-NM-05**: Graceful shutdown — verify daemon shuts down cleanly on service stop [FR-E07]

### Host Integration Tests (Go)
- **T-HI-01**: SSH agent protocol — verify list-keys and sign-request with a mock phone TLS server [FR-001, FR-004, FR-005, FR-006]
- **T-HI-02**: mTLS handshake — verify mutual certificate validation and pinning [FR-010, FR-011]
- **T-HI-03**: Pairing flow — verify full pairing with temporary HTTPS server and simulated phone [FR-020 through FR-028]
- **T-HI-04**: CLI commands — verify all CLI commands against a running daemon [FR-070 through FR-077]
- **T-HI-05**: Timeout behavior — verify connectionTimeout and signTimeout [FR-E01, FR-E03]
- **T-HI-06**: Key listing with allowKeyListing disabled [FR-005, FR-066]
- **T-HI-07**: Config validation — verify fail-fast on invalid config [FR-100, FR-E06]
- **T-HI-08**: Concurrent sign requests [FR-E04]
- **T-HI-09**: OTEL trace propagation — verify traceparent in mTLS headers [FR-082, FR-083, FR-085]
- **T-HI-10**: IP update on reconnect — verify stored IP updates when phone's Tailscale IP changes [FR-E12]

### Android Instrumented Tests
- **T-AI-01**: Key generation — create Ed25519 and ECDSA keys in Keystore [FR-040, FR-041]
- **T-AI-02**: Key non-extractability — verify private key cannot be exported [FR-042]
- **T-AI-03**: Public key export — verify SSH format output [FR-043, FR-044]
- **T-AI-04**: Confirmation policies — verify biometric/password prompts per policy [FR-045, FR-051]
- **T-AI-05**: TLS server lifecycle — verify server starts/stops with app foreground/background [FR-013, FR-014]
- **T-AI-06**: Sign request handling — verify prompt display and signing [FR-050, FR-053]
- **T-AI-07**: Key listing denial — verify phone-side deny overrides host [FR-054]
- **T-AI-08**: Pairing flow — verify QR decode, confirmation prompt, cert storage [FR-022, FR-024, FR-026]
- **T-AI-09**: OTEL acceptance — verify tracing config prompt and storage [FR-023, FR-088]
- **T-AI-10**: Key deletion — verify biometric confirmation required [FR-047]

### End-to-End Tests (Headscale-backed)
- **T-E2E-00**: All E2E tests MUST use a headscale instance (self-hosted Tailscale control server) spun up in the NixOS VM test. This provides a real Tailnet without requiring external Tailscale accounts. Both the host daemon and simulated phone client authenticate against headscale with pre-authorized keys. [FR-013a, FR-013b]
- **T-E2E-01**: Full signing flow — NixOS VM with headscale + daemon + simulated phone on the same Tailnet. SSH client requests signature, phone signs, SSH operation succeeds [Story 1]
- **T-E2E-02**: Full pairing flow — NixOS VM with headscale runs `nix-key pair`, simulated phone completes pairing over Tailnet [Story 2]
- **T-E2E-03**: Distributed trace — signing flow with OTEL, verify trace appears in collector with correct span hierarchy [Story 7]

---

## Success Criteria

- **SC-001**: SSH signing via phone works end-to-end — `ssh-add -L` lists keys, SSH operations succeed when phone approves [validates FR-001 through FR-007, FR-050 through FR-053]
- **SC-002**: Pairing completes with mutual confirmation and certificate exchange [validates FR-020 through FR-030]
- **SC-003**: Keys are created in Android Keystore, public key exports in valid SSH format [validates FR-040 through FR-048]
- **SC-004**: NixOS module installs cleanly via flakes and channels, service starts, SSH_AUTH_SOCK is set [validates FR-060 through FR-069]
- **SC-005**: All CLI commands work correctly against a running daemon [validates FR-070 through FR-077]
- **SC-006**: Phone unreachable / user deny / timeout all result in SSH_AGENT_FAILURE [validates FR-E01 through FR-E03]
- **SC-007**: mTLS with certificate pinning enforced on all connections [validates FR-010 through FR-016]
- **SC-008**: Distributed traces visible in Jaeger when OTEL is enabled [validates FR-080 through FR-088]
- **SC-009**: Zero critical vulnerabilities in security scan [validates scanning pipeline]
- **SC-010**: NixOS VM tests pass — service lifecycle, config generation, device merge [validates T-NM-01 through T-NM-05]

---

## Assumptions

- User has a Tailscale (or headscale) account and both devices (NixOS machine + Android phone) can join the same Tailnet
- Phone requires a Tailscale auth key or OAuth flow on first launch to join the Tailnet
- NixOS machine is running a recent NixOS (24.05+) with flake support
- Android device runs Android 10+ (API 29+) for Keystore hardware backing
- User has physical proximity to both devices during pairing (QR code scanning)
- SSH keys are not recoverable — factory reset or phone loss means re-generating keys (YubiKey model)
- Single user per host — no multi-user SSH agent sharing
- The Tailscale userspace implementation for Android supports listening on the Tailscale interface
