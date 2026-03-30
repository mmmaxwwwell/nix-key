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
5. **Given** a key is locked with password unlock policy and biometric signing policy, **When** a sign request arrives, **Then** the phone prompts for password (unlock), then BiometricPrompt (signing), then signs.
6. **Given** an unlocked key with biometric signing policy, **When** a second sign request arrives, **Then** only BiometricPrompt is shown (no unlock prompt — already unlocked).
7. **Given** an unlocked key with auto-approve signing policy, **When** a sign request arrives, **Then** the phone signs immediately without user interaction and returns the signature.

---

### User Story 2 — Device Onboarding / Pairing (Priority: P1)

A user runs `nix-key pair` on their NixOS machine. The CLI spins up a temporary HTTPS server bound to the Tailscale interface and displays a QR code in the terminal. The QR code contains the host's Tailscale IP, temporary pairing port, full TLS server certificate (for immediate pinning), one-time pairing token, and optionally the OTEL collector endpoint. The user opens the nix-key Android app, scans the QR code, and the app asks "Connect to `hostname`? [Accept/Deny]". If the host has OTEL configured, the app also prompts "Enable tracing? Traces will be sent to `100.x.x.x:4317` [Accept/Deny]". On accept, the phone connects to the temporary HTTPS endpoint and presents its own TLS certificate, Tailscale IP, and listening port. The host CLI shows "Phone `Pixel 8` wants to pair. Authorize? [y/N]". On mutual confirmation, both sides store each other's certificates and connection info. The temporary HTTPS server shuts down.

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

A user opens the nix-key app and creates a new SSH key. They choose a name, key type (Ed25519 default, ECDSA-P256), unlock policy (password default), and signing policy (biometric default). The key is generated inside Android Keystore (hardware-backed when available). The public key is displayed and can be exported via clipboard, share sheet, or QR code.

**Why this priority**: Keys must exist before signing can work. Co-P1 with Stories 1 and 2.

**Independent Test**: Android instrumented test that creates a key, verifies it exists in Keystore, exports the public key, and verifies the exported format is valid SSH public key format.

**Acceptance Scenarios**:

1. **Given** the user taps "Create Key", **When** they fill in name and select Ed25519, **Then** a key is generated in Android Keystore.
2. **Given** a key exists, **When** the user taps "Export Public Key", **Then** the public key is available in standard `ssh-ed25519 AAAA... name` format.
3. **Given** a key exists, **When** the user sets unlock policy to "password" and signing policy to "biometric", **Then** the key requires password to unlock and fingerprint for each sign request.
4. **Given** a key has auto-approve signing or none-unlock selected, **Then** the app shows a warning about the security implications before saving.
5. **Given** a locked key, **When** a sign request arrives, **Then** the app prompts for the unlock policy first, then the signing policy.
6. **Given** an unlocked key, **When** the app is backgrounded and foregrounded, **Then** the key remains unlocked.
7. **Given** an unlocked key, **When** the app process is killed and relaunched, **Then** the key is locked again.
8. **Given** the user creates a key, **Then** the private key material is never extractable from Android Keystore.

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

### Non-Goals

These are things nix-key deliberately does NOT do, even though users might reasonably expect them:

1. **No RSA keys** — Legacy, larger, slower. Ed25519 and ECDSA-P256 cover all modern SSH use cases.
2. **No key export or backup** — YubiKey model. Private keys are non-extractable. Lose the phone = re-generate keys and update `authorized_keys`. "Rotate your shit."
3. **No host-side agent lock/unlock** — The SSH agent protocol's lock (`ssh-add -x`) and unlock (`ssh-add -X`) are not implemented. Phone reachability (app open/closed) serves as the coarse lock. Per-key unlock on the phone (FR-116-119) is a separate, finer-grained mechanism for decrypting key material into memory.
4. **No persistent Tailscale connection** — Userspace Tailscale only when the app is foregrounded. Battery and security benefit. No background service keeping the phone on the Tailnet.
5. **No Windows or macOS host support** — NixOS exclusive. The NixOS module is the primary distribution mechanism. Non-NixOS Linux could work manually but is not a goal.
6. **No multi-user SSH agent sharing** — Single user per host. The systemd user service runs per-user, not system-wide.
7. **No Diffie-Hellman key exchange for pairing** — QR-based cert exchange with physical proximity is simpler and sufficient. No TOFU vulnerability.
8. **No auto-retry on phone unreachable** — Fail fast. SSH clients handle their own retries.
9. **No timeout-based auto-approve** — No "approve for 5 minutes" mode. Each sign request requires explicit approval (or auto-approve per key, with warning). Simplicity over convenience.
10. **No Docker for development** — Nix-first project. `nix develop` provides everything. No Docker Compose, no devcontainers.

---

### Operational Workflows

**Day-1 Setup (first 10 minutes):**
1. Add `nix-key` flake input → `services.nix-key.enable = true` with config → `nixos-rebuild switch`
2. Verify: `systemctl --user status nix-key-agent` shows active
3. `nix-key pair` → scan QR code on phone → mutual confirm on both sides
4. `ssh-add -L` → see phone's keys listed
5. `ssh user@host` → approve on phone → SSH succeeds

**Day-2 Operations:**
- Add another phone: `nix-key pair` (supports multiple phones)
- Check status: `nix-key status` (daemon state, connected devices, cert expiry warnings)
- Revoke a lost phone: `nix-key revoke <device>` (removes cert, removes from registry)
- Debug connectivity: `nix-key test <device>` → `nix-key logs`
- Create/manage keys: done entirely on the phone app
- Rotate certs: re-pair the device (no in-place cert rotation — re-pairing is the rotation mechanism)
- View config: `nix-key config`
- Export a key: `nix-key export <key-id>` (prints SSH public key to stdout)

**Failure Recovery:**
- Phone unreachable → is the app open? → `nix-key test <device>` → check `nix-key logs`
- Daemon crash → `journalctl --user -u nix-key-agent` → `systemctl --user restart nix-key-agent` → `nix-key status`
- Cert approaching expiry → `nix-key status` warns at 30 days → re-pair to rotate
- Lost phone → `nix-key revoke <device>` → re-generate keys on new phone → update `authorized_keys` on servers

**Admin Processes:**
- Cert rotation: re-pair the device
- Clear stale devices: `nix-key devices` → `nix-key revoke` for offline devices
- Enable tracing for debugging: set `services.nix-key.tracing.otelEndpoint` → rebuild → reproduce issue → view traces in Jaeger
- Reset all state: `rm -rf ~/.config/nix-key/ ~/.local/state/nix-key/` → re-pair all devices

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
- **FR-E13**: When a key unlock attempt fails (wrong password, biometric failure after Android's standard retry limit), the sign request MUST fail with SSH_AGENT_FAILURE. The key remains locked. No custom retry logic — Android's BiometricPrompt handles retries and lockout.
- **FR-E14**: When age decryption of mTLS private keys fails at daemon startup (wrong age identity, corrupted encrypted file), the daemon MUST refuse to start with a clear error message identifying which file failed and why. This is distinct from FR-E06 (config validation) — config may be valid but the referenced encrypted files may not be.
- **FR-E15**: When a host sends a sign request for a key that has been deleted on the phone, the phone MUST return a gRPC error indicating the key was not found. The daemon MUST return SSH_AGENT_FAILURE to the SSH client.
- **FR-E16**: When `nix-key pair` fails mid-write (disk full, permission error), no partial state MUST be persisted. The pairing MUST be atomic — either both sides have complete state or neither does. On write failure, clean up any partial files and report the error.
- **FR-E17**: When `nix-key pair` is run while another pairing session is already in progress, the second invocation MUST fail with "Pairing already in progress" rather than a cryptic port-binding error.
- **FR-E18**: When an mTLS certificate expires while the daemon is running, existing connections are NOT disrupted (certs are validated at handshake time, not mid-stream). New connections to/from that device will fail with a cert validation error. The daemon MUST log a WARN when a connection fails due to an expired cert and suggest re-pairing.
- **FR-E19**: When the phone's gRPC server fails to bind its configured port (port in use, permission denied), the app MUST show a specific error notification (e.g., "Port 29418 in use") rather than a generic "failed to start" message. The Tailnet connection indicator (FR-110) MUST show red/"Disconnected".

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
- **FR-032**: mTLS certificates MUST have a configurable expiry (default: no expiry). Configurable via `services.nix-key.certExpiry` (NixOS module, applies to all future pairings) or `nix-key pair --expiry <duration>` (CLI flag, overrides module setting for this pairing). When a cert has an expiry set, `nix-key status` MUST warn when a device cert is within 30 days of expiry. Re-pairing is required to rotate certs.
- **FR-031**: `nix-key pair` MUST generate two certificate pairs: (a) a TLS server cert/key for the temporary pairing HTTPS server, and (b) a TLS client cert/key that the host will use during normal operation to connect to phones.

### Functional Requirements — Key Management (Android)

- **FR-040**: The app MUST generate SSH keys with private key material protected by Android Keystore. ECDSA-P256 keys MUST use native Keystore hardware backing (TEE/StrongBox) when available. Ed25519 keys MUST be software-generated (via BouncyCastle) and encrypted at rest with a Keystore-backed AES wrapping key, since Android Keystore does not natively support Ed25519.
- **FR-041**: Supported key types: Ed25519 (default, software + Keystore-wrapped), ECDSA-P256 (native Keystore hardware-backed).
- **FR-042**: Private key material MUST never be extractable from Android Keystore.
- **FR-043**: Public keys MUST be exportable in standard SSH format (`ssh-ed25519 AAAA... name`).
- **FR-044**: Export methods: clipboard, share sheet, QR code.
- **FR-045**: Each key MUST have two independent, user-configurable policies:
  - **Unlock policy** (decrypt key material into memory, once per app session): none (auto-unlock on app start), biometric, password (default), biometric+password.
  - **Signing policy** (per sign request, after key is unlocked): always-ask (simple Approve/Deny dialog, no biometric or password — just a tap), biometric (default — BiometricPrompt), password (device credential prompt), biometric+password (both required), auto-approve (no dialog, sign immediately).
  Example: a key with "password unlock, biometric signing" requires a password once when the app opens, then fingerprint for each SSH sign request.
- **FR-046**: Auto-approve signing policy AND none-unlock policy MUST each show a security warning before the user can enable them.
- **FR-047**: Key deletion MUST require biometric or password confirmation.
- **FR-048**: Each key MUST have a user-editable display name.

### Functional Requirements — Sign Request Handling (Android)

- **FR-050**: When a sign request arrives and the key is unlocked (or after unlock succeeds per FR-116), the app MUST show a sign confirmation overlay/dialog with: host name, key name, data hash (truncated) — unless the signing policy is auto-approve, in which case the app signs immediately with no dialog.
- **FR-051**: The sign request flow MUST execute the key's two policies in order: (1) if the key is locked, trigger the unlock policy (biometric/password/both/none per FR-116/FR-119), then (2) trigger the signing policy (always-ask/biometric/password/both/auto-approve per FR-045).
- **FR-052**: [REMOVED — SSH agent protocol always includes a key blob in sign requests; this scenario is unreachable. When `allowKeyListing` is disabled, users must configure their SSH client with the correct public key via `IdentityFile`.]
- **FR-053**: Multiple concurrent sign requests MUST be queued and shown as separate prompts. If the key is locked, the first queued request triggers the unlock prompt; subsequent queued requests for the same key wait and skip unlock once the first request unlocks it. If the first request's unlock fails (FR-E13), the next queued request triggers its own unlock prompt — giving the user another chance to authenticate.
- **FR-054**: The phone MUST have a setting to deny key listing regardless of host configuration.

### Functional Requirements — Key Unlock Lifecycle (Android)

- **FR-116**: When a sign request arrives for a locked key (key material not yet decrypted into memory), the app MUST automatically trigger the key's unlock prompt before proceeding to the signing policy prompt. The unlock prompt MUST show the key name and the requesting host name (e.g., "Unlock key 'work-key' for host 'laptop'?") so the user knows why they're unlocking.
- **FR-117**: The app MUST provide a way to manually re-lock a key from the UI (lock button on key detail screen or long-press action on key list). Re-locking wipes the decrypted key material from memory. If sign requests are queued for the re-locked key, the next queued request triggers a fresh unlock prompt.
- **FR-118**: Decrypted key material MUST remain in memory while the app process is alive, including when the app is backgrounded. When the app process is killed (by the OS or user), all decrypted key material MUST be wiped — keys return to locked state on next app launch.
- **FR-119**: The unlock prompt MUST use the key's configured unlock policy (biometric, password, biometric+password, or none). If the unlock policy is "none", the key is eagerly decrypted on app start with no user interaction (no waiting for first sign request). Keys with any other unlock policy remain locked until a sign request triggers FR-116.

### Functional Requirements — NixOS Module

- **FR-060**: The module MUST be importable via `nix-key.nixosModules.default` (flakes) or `fetchTarball` (channels).
- **FR-061**: `services.nix-key.enable` MUST create a systemd user service `nix-key-agent.service`.
- **FR-062**: The module MUST write all configuration to `~/.config/nix-key/config.json`.
- **FR-063**: The module MUST support declarative device definitions in the `devices` attrset.
- **FR-064**: Declarative devices MUST be merged with runtime-paired devices from `~/.local/state/nix-key/devices.json`. On conflict (same cert fingerprint with different values), Nix-declared values take precedence. Nix-declared fields set to `null` are filled by runtime-paired values.
- **FR-065**: Each device in the `devices` attrset MUST support optional `clientCert` and `clientKey` paths for programmatic mTLS cert definition, with `null` meaning "set by pairing".
- **FR-066**: The `allowKeyListing` option MUST include documentation noting that the phone can also independently deny listing, resulting in an empty array.
- **FR-067**: The module MUST support `services.nix-key.tracing.otelEndpoint` for OTEL collector configuration.
- **FR-068**: The module MUST support `services.nix-key.tracing.jaeger.enable` to optionally run a local Jaeger instance.
- **FR-069**: The flake MUST export: `nixosModules.default`, `packages.default` (CLI), `checks` (all tests), `overlays.default`.
- **FR-069a**: The module MUST create `~/.config/environment.d/50-nix-key.conf` containing `SSH_AUTH_SOCK=<socketPath>` so all user login sessions pick up the agent socket path automatically.

### Functional Requirements — CLI

- **FR-070**: `nix-key pair` MUST display a QR code in the terminal and wait for phone connection.
- **FR-071**: `nix-key devices` MUST list all authorized phones with name, Tailscale IP, cert fingerprint, last seen, connection status.
- **FR-072**: `nix-key revoke <device>` MUST remove the device and delete its cert. Revocation is host-side only — the phone is not notified. The phone discovers revocation when its next mTLS connection is rejected (FR-E09).
- **FR-073**: `nix-key status` MUST show daemon status, connected devices, available keys, socket path.
- **FR-074**: `nix-key export <key-id>` MUST print the public key in SSH format to stdout. `key-id` is the key's SHA256 fingerprint (as shown by `nix-key status`) or a unique prefix match. If the prefix matches multiple keys, error with "ambiguous prefix" and list candidates. The phone holding the key MUST be reachable — export queries the phone via gRPC, no local key caching.
- **FR-075**: `nix-key config` MUST show current configuration.
- **FR-076**: `nix-key logs` MUST tail daemon logs with human-readable formatting.
- **FR-077**: `nix-key test <device>` MUST verify mTLS handshake and round-trip to the specified phone.

### Functional Requirements — CLI ↔ Daemon Communication

- **FR-078**: The daemon MUST expose a control socket (Unix socket at a configurable `controlSocketPath`) for CLI commands to communicate with the running daemon.
- **FR-079**: The control socket protocol MUST use line-delimited JSON. Commands: `register-device`, `list-devices`, `revoke-device`, `get-status`, `get-keys`. Each request is a JSON object with a `command` field; each response is a JSON object with a `status` field and command-specific data.

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

### Functional Requirements — Android UI Status & Loading States

- **FR-110**: The Android app MUST display a persistent Tailnet connection indicator visible across all screens: green/"Connected" when Tailscale is up and gRPC server is listening, yellow/"Connecting..." during Tailscale auth or reconnect, red/"Disconnected" when Tailscale is not started or auth failed.
- **FR-111**: The key list screen MUST show a per-key lock/unlock indicator reflecting whether the key's material is currently decrypted in memory: locked icon when the key has not been unlocked this session, unlocked icon when the key material is in memory and ready to sign. Keys in auto-approve signing mode with none-unlock policy show the unlocked icon with a security warning badge (per FR-046).
- **FR-112**: During Tailscale auth (first launch or reconnect), the app MUST show "Connecting to Tailnet..." with a progress indicator. On timeout (30s) or failure, show an error message with retry button. Never show "ready" while auth is in progress.
- **FR-113**: During QR pairing flow, the app MUST show sequential states: "Scanning..." during QR decode, "Connecting to host..." during pairing POST, "Waiting for host approval..." while the HTTP connection is held open. On timeout or failure, show actionable error with retry.
- **FR-114**: The gRPC server startup notification MUST show "Starting nix-key..." during Tailscale + gRPC initialization. The "nix-key active" notification MUST only appear after the gRPC server is actually listening and ready to serve requests.
- **FR-115**: If Tailscale auth state is stale on app launch (e.g., token expired), the app MUST show the re-auth flow instead of crashing or showing a blank screen.

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
- **SSHKey**: An SSH key on a phone. Attributes: public key blob, key type, display name, fingerprint, unlock policy (none/biometric/password/biometric+password, default: password), signing policy (always-ask/biometric/password/biometric+password/auto-approve, default: biometric), unlock state (locked/unlocked, runtime only — not persisted). Note: private key exists only in Android Keystore. Unlock state resets to locked on process kill.
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
- **Secrets**: mTLS private keys stored encrypted at rest in `~/.local/state/nix-key/certs/` using age encryption. Decrypted into memory at daemon startup. Config references encrypted file paths, never plaintext keys. File permissions 0600 as defense-in-depth.

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
- **Additional Makefile targets**: `make bench` (microbenchmarks), `make security-scan` (local security scan with JSON output), `make validate` (full local CI parity: test + lint + security scan)
- **Debugging**: VS Code `launch.json` for Go daemon, Android Studio for the app
- **Concurrency safety**: Go tests run with `-race` flag. Android code uses `@GuardedBy`/`@ThreadSafe` annotations checked by Infer/RacerD in CI.
- **Fuzz testing**: Go native fuzzing (`testing.F`) for protocol parsers. Run on PRs and develop pushes (time-boxed). Seed corpus committed as regression tests.

### Security Scanning (Local)
- **Makefile target**: `make security-scan` runs Trivy, Semgrep, Gitleaks, govulncheck with JSON output to `test-logs/security/`
- **Summary**: Aggregated `test-logs/security/summary.json` with per-scanner finding counts and pass/fail
- **CI dual output**: SARIF (for GitHub Security tab) + JSON (for agent debugging) from each scanner
- **`make validate`**: Full local CI parity — `make test && make lint && make security-scan`

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
- **T-NM-06**: Log level config — verify `services.nix-key.logLevel = "DEBUG"` changes daemon log output level [FR-092]
- **T-NM-07**: Declarative device cert paths — verify devices with `clientCert`/`clientKey` paths work correctly for mTLS [FR-065]
- **T-NM-08**: Environment.d — verify `~/.config/environment.d/50-nix-key.conf` is created with correct SSH_AUTH_SOCK [FR-069a]

### Host Integration Tests (Go)
- **T-HI-01**: SSH agent protocol — verify list-keys (including skip unreachable phones after connectionTimeout) and sign-request with a mock phone TLS server [FR-001, FR-004, FR-005, FR-006, FR-007]
- **T-HI-02**: mTLS handshake — verify mutual certificate validation and pinning [FR-010, FR-011]
- **T-HI-03**: Pairing flow — verify full pairing with temporary HTTPS server and simulated phone [FR-020 through FR-028]
- **T-HI-04**: CLI commands — verify all CLI commands against a running daemon [FR-070 through FR-077]
- **T-HI-05**: Timeout behavior — verify connectionTimeout and signTimeout, mid-connection drop returns failure [FR-E01, FR-E03, FR-E08]
- **T-HI-06**: Key listing with allowKeyListing disabled [FR-005, FR-066]
- **T-HI-07**: Config validation — verify fail-fast on invalid config [FR-100, FR-E06]
- **T-HI-08**: Concurrent sign requests [FR-E04]
- **T-HI-09**: OTEL trace propagation — verify traceparent in mTLS headers [FR-082, FR-083, FR-085]
- **T-HI-09a**: Tracing disabled = no overhead — verify no tracer initialized, no span creation, no OTLP connections when otelEndpoint is null [FR-086]
- **T-HI-10**: IP update on reconnect — verify stored IP updates when phone's Tailscale IP changes [FR-E12]
- **T-HI-11**: Cert expiry warning — verify `nix-key status` warns when device cert is within 30 days of expiry [FR-032]
- **T-HI-12**: Two cert pairs generated during pairing — verify `nix-key pair` generates a server cert (temp HTTPS) and a client cert (normal operation), both distinct [FR-031]
- **T-HI-13**: Age decryption failure — verify daemon refuses to start with clear error when age identity is wrong or encrypted file is corrupted [FR-E14]
- **T-HI-14**: Deleted key sign request — verify SSH_AGENT_FAILURE when host requests signing with a key fingerprint that no longer exists on the phone [FR-E15]
- **T-HI-15**: Atomic pairing — verify no partial state persisted when pairing write fails (simulate disk error) [FR-E16]
- **T-HI-16**: Concurrent pairing rejection — verify second `nix-key pair` fails with "Pairing already in progress" [FR-E17]
- **T-HI-17**: Error hierarchy — verify typed errors include error codes and correlation IDs, SSH client errors are sanitized [FR-095, FR-096, FR-097]
- **T-HI-18**: Logging — verify structured JSON output, correct levels, correlation ID propagation, security events at INFO+ [FR-090, FR-091, FR-094]
- **T-HI-19**: Secrets at rest — after pairing, verify private key files on disk are age-encrypted (not plaintext PEM), verify daemon decrypts into memory and never writes plaintext to any file, verify `ageKeyFile` config option works [FR-101, FR-102, FR-103, FR-104]
- **T-HI-20**: Control socket — verify daemon creates control socket at configured path, CLI can connect and exchange JSON commands [FR-078, FR-079]
- **T-HI-21**: Pairing without Tailscale — verify `nix-key pair` fails with clear error when Tailscale interface is unavailable [FR-E11]

### Android Instrumented Tests
- **T-AI-01**: Key generation — create Ed25519 and ECDSA keys in Keystore [FR-040, FR-041]
- **T-AI-02**: Key non-extractability — verify private key cannot be exported [FR-042]
- **T-AI-03**: Public key export — verify SSH format output [FR-043, FR-044]
- **T-AI-04**: Unlock and signing policies — verify independent unlock policy (biometric/password/biometric+password/none) and signing policy (always-ask/biometric/password/biometric+password/auto-approve) per key. Test: password-unlock + biometric-sign combo, biometric+password combo for both policies, auto-unlock with warning, sign request on locked key triggers unlock first [FR-045, FR-051, FR-116, FR-119]
- **T-AI-11**: Key unlock lifecycle — verify key material persists in memory across background/foreground, wiped on process kill, manual re-lock via UI [FR-117, FR-118]
- **T-AI-05**: TLS server lifecycle — verify server starts/stops with app foreground/background [FR-013, FR-014]
- **T-AI-06**: Sign request handling — verify prompt display, signing on approve, SSH_AGENT_FAILURE on deny [FR-050, FR-053, FR-E02]
- **T-AI-07**: Key listing denial — verify phone-side deny overrides host [FR-054]
- **T-AI-08**: Pairing flow — verify QR decode, confirmation prompt, cert storage [FR-022, FR-024, FR-026]
- **T-AI-09**: OTEL acceptance — verify tracing config prompt and storage [FR-023, FR-088]
- **T-AI-10**: Key deletion — verify biometric confirmation required [FR-047]
- **T-AI-12**: Unlock failure — verify failed unlock (wrong password, biometric failure) results in sign request failure, key remains locked [FR-E13]
- **T-AI-13**: Security warnings — verify warning dialogs shown when enabling auto-approve signing or none-unlock policy [FR-046]
- **T-AI-14**: Display name editing — verify key display name can be edited and persists [FR-048]
- **T-AI-15**: Android structured logging — verify JSON log output, trace ID correlation when OTEL enabled, security events at INFO+ [FR-093, FR-094]
- **T-AI-16**: Expired cert mid-session — verify new connections fail with expired cert, existing connections unaffected, WARN logged [FR-E18]
- **T-AI-17**: gRPC port conflict — verify specific error notification when port unavailable, Tailnet indicator shows red [FR-E19]
- **T-AI-18**: Multi-host pairing — pair phone with two mock hosts, verify both stored correctly, sign requests from each host work independently [FR-030]

### Adversarial Security Tests (NixOS VM)
- **T-ADV-01**: Rogue node with expired client cert attempts mTLS connection → rejected [FR-E05, FR-011]
- **T-ADV-02**: Rogue node with cert signed by different CA → rejected [FR-011]
- **T-ADV-03**: Rogue node with valid-but-unpaired cert (not in devices.json) → rejected [FR-E09]
- **T-ADV-04**: Connection attempt on non-Tailscale interface (raw eth0) → rejected [FR-015]
- **T-ADV-05**: Replayed one-time pairing token → rejected [FR-E10]
- **T-ADV-06**: Verify error responses from adversarial connections leak no internal details [FR-097]

### Fuzz Tests (Go native fuzzing)
- **T-FZ-01**: SSH agent protocol message parsing [FR-001]
- **T-FZ-02**: Protobuf message deserialization [FR-017]
- **T-FZ-03**: QR code payload JSON parsing [FR-021]
- **T-FZ-04**: Config JSON parsing [FR-100]
- **T-FZ-05**: Certificate PEM parsing [FR-010]
- **T-FZ-06**: Control socket JSON protocol parsing [FR-079]

### Performance & Latency Tests
- **T-PF-01**: E2E sign request latency — 20 runs, p95 < 2 seconds [Performance goal]
- **T-PF-02**: mTLS handshake microbenchmark — < 200ms [Performance sub-budget]
- **T-PF-03**: gRPC round-trip microbenchmark — < 100ms [Performance sub-budget]
- **T-PF-04**: Age decrypt microbenchmark — < 50ms [Performance sub-budget]

### QR Scanning Test
- **T-QR-01**: ML Kit `InputImage.fromBitmap()` test — feed QR code bitmap directly to barcode scanner, verify correct payload extraction without camera hardware [FR-022, FR-021]

### Android UI Status Tests
- **T-UI-01**: Tailnet connection indicator shows correct states: disconnected → connecting → connected [FR-110]
- **T-UI-02**: Per-key lock/unlock indicators reflect runtime unlock state (locked = key material not in memory, unlocked = decrypted) [FR-111, FR-118]
- **T-UI-03**: Loading states during Tailscale auth, pairing, and gRPC startup — never show "ready" while init in progress [FR-112, FR-113, FR-114]
- **T-UI-04**: Stale Tailscale auth triggers re-auth flow, not crash [FR-115]

### DX Makefile Target Tests
- **T-DX-01**: `make test` runs unit + integration tests with structured output to `test-logs/`, exits 0 on pass, non-zero on fail [DX]
- **T-DX-02**: `make test-unit` runs only `-short` tests, `make test-integration` runs only `Integration` tests [DX]
- **T-DX-03**: `make lint` runs golangci-lint, exits non-zero on findings [DX]
- **T-DX-04**: `make build` produces a working `nix-key` binary that responds to `--help` [DX]
- **T-DX-05**: `make proto` regenerates protobuf Go code without errors [DX]
- **T-DX-06**: `make bench` runs microbenchmarks and prints results [DX]
- **T-DX-07**: `make security-scan` runs all scanners, writes JSON to `test-logs/security/`, produces `summary.json` [DX]
- **T-DX-08**: `make validate` runs test + lint + security-scan as a single command, exits 0 only if all pass [DX]
- **T-DX-09**: `make cover` generates HTML coverage report in `coverage/` [DX]
- **T-DX-10**: `make generate-fixtures` regenerates deterministic test fixtures without errors [DX]
- **T-DX-11**: `make clean` and `make clean-all` remove expected artifacts without errors [DX]

### Cold-Start & Idempotency Tests
- **T-CS-01**: Delete all state dirs (`~/.config/nix-key/`, `~/.local/state/nix-key/`), start daemon with valid config → verify dirs created with correct permissions (0700 for secret dirs), daemon starts successfully [Idempotency, Operational Workflows]
- **T-CS-02**: Stop daemon, start again (warm start) → verify reuses existing state, no errors, no re-creation of existing artifacts [Idempotency]
- **T-CS-03**: Run `nix-key pair` setup twice → verify second run doesn't corrupt state, age identity is skip-if-exists [Idempotency, FR-104]

### End-to-End Tests (Headscale-backed)
- **T-E2E-00**: All E2E tests MUST use a headscale instance (self-hosted Tailscale control server) spun up in the NixOS VM test. This provides a real Tailnet without requiring external Tailscale accounts. Both the host daemon and simulated phone client authenticate against headscale with pre-authorized keys. [FR-013a, FR-013b]
- **T-E2E-01**: Full signing flow — NixOS VM with headscale + daemon + simulated phone on the same Tailnet. SSH client requests signature, phone signs, SSH operation succeeds [Story 1]
- **T-E2E-02**: Full pairing flow — NixOS VM with headscale runs `nix-key pair`, simulated phone completes pairing over Tailnet [Story 2]
- **T-E2E-03**: Distributed trace — signing flow with OTEL, verify trace appears in collector with correct span hierarchy [Story 7]

---

## Success Criteria

- **SC-001**: SSH signing via phone works end-to-end — `ssh-add -L` lists keys, SSH operations succeed when phone approves (including unlock-then-sign flow for locked keys) [validates FR-001 through FR-007, FR-050 through FR-053, FR-116 through FR-119]
- **SC-002**: Pairing completes with mutual confirmation and certificate exchange, generates two cert pairs, certs have configurable expiry [validates FR-020 through FR-032]
- **SC-003**: Keys are created in Android Keystore, public key exports in valid SSH format [validates FR-040 through FR-048]
- **SC-004**: NixOS module installs cleanly via flakes and channels, service starts, SSH_AUTH_SOCK is set via environment.d [validates FR-060 through FR-069a]
- **SC-005**: All CLI commands work correctly against a running daemon via control socket [validates FR-070 through FR-079]
- **SC-006**: Phone unreachable / user deny / timeout all result in SSH_AGENT_FAILURE [validates FR-E01 through FR-E03]
- **SC-007**: mTLS with certificate pinning enforced on all connections [validates FR-010 through FR-016]
- **SC-008**: Distributed traces visible in Jaeger when OTEL is enabled [validates FR-080 through FR-088]
- **SC-009**: Zero critical vulnerabilities in security scan [validates scanning pipeline]
- **SC-010**: NixOS VM tests pass — service lifecycle, config generation, device merge, log level, declarative certs, environment.d [validates T-NM-01 through T-NM-08]
- **SC-011**: Adversarial connections rejected — expired cert, wrong CA, unpaired device, non-Tailscale interface, replayed token all fail with no internal detail leakage [validates T-ADV-01 through T-ADV-06]
- **SC-012**: Fuzz targets run clean — seed corpus passes as regression, no crashes on generative fuzzing [validates T-FZ-01 through T-FZ-06]
- **SC-013**: Sign request p95 latency < 2 seconds with simulated phone [validates T-PF-01]
- **SC-014**: Android app shows correct Tailnet status and per-key lock indicators, loading states for all async operations [validates FR-110 through FR-115]
- **SC-015**: QR code scanning works via ML Kit image input (bitmap test) [validates T-QR-01]
- **SC-016**: README.md exists with install instructions, config table, CI setup, architecture diagram [validates Documentation]
- **SC-017**: MIT license (or least restrictive compatible with dependencies) [validates License]
- **SC-018**: Structured JSON logging on host and phone, correct log levels, correlation IDs propagated, security events logged at INFO+ [validates FR-090 through FR-094]
- **SC-019**: Typed error hierarchy with error codes and correlation IDs, sanitized SSH client errors [validates FR-095 through FR-097]
- **SC-020**: Config fail-fast on invalid input, age encryption for all mTLS private keys at rest, decryption into memory only [validates FR-098 through FR-104]
- **SC-021**: Cold-start creates dirs with correct permissions, warm-start reuses state, pairing is idempotent [validates T-CS-01 through T-CS-03]
- **SC-022**: All Makefile targets work correctly (test, lint, build, bench, security-scan, validate, cover, proto, generate-fixtures, clean) [validates T-DX-01 through T-DX-11]

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
