# Interview Notes — nix-key

**Date**: 2026-03-28
**Preset**: public
**Nix available**: yes

## Key Decisions

### Architecture
- **Stack**: Go (host daemon + CLI), Kotlin/Jetpack Compose (Android app)
- **Connection direction**: Host → Phone for signing (phone is TLS server). Phone → Host only during pairing.
- **Tailscale**: Userspace implementation on Android — phone is only on Tailnet when app is foregrounded. Host uses system Tailscale.
- **Rationale**: Go chosen for excellent SSH agent protocol libraries (`golang.org/x/crypto/ssh/agent`), single static binary, trivial Nix builds. Kotlin is the natural choice for Android with Keystore and BiometricPrompt.

### SSH Agent Protocol
- **Implemented operations**: List keys (`SSH2_AGENTC_REQUEST_IDENTITIES`), Sign (`SSH2_AGENTC_SIGN_REQUEST`)
- **Not implemented**: Add key, Remove key, Lock/Unlock
- **Rationale**: Keys are created and managed exclusively on the phone. Lock/unlock unnecessary — phone's Tailnet presence serves as the lock. If app is closed, agent is unreachable.

### Key Management
- **Key types**: Ed25519 (default), ECDSA-P256. No RSA.
- **Storage**: Android Keystore, hardware-backed when available, never extractable
- **Backup model**: YubiKey model — no backup, no recovery. Lose the phone = re-generate keys.
- **User explicitly chose**: "rotate your shit" — no software-backed exportable key option.

### Confirmation Policies
- Per-key configurable: always ask (default), biometric only, password only, biometric+password, auto-approve (with warning)
- No timeout-based auto-approve (e.g., "approve for 5 minutes") — decided to keep it simple

### Pairing Flow
- **Direction**: Phone initiates connection to host's temporary HTTPS endpoint during pairing
- **QR code contents**: Host Tailscale IP, temp port, TLS cert fingerprint, one-time token, optional OTEL endpoint
- **Mutual confirmation**: Both sides must explicitly confirm
- **QR code**: Contains host's full server cert (not just fingerprint) for immediate pinning. Phone generates its own cert and sends it during pairing handshake.
- **Simple approach**: Cert exchange over the pairing HTTPS connection — accepted because pairing requires physical proximity
- **Multi-device**: Multiple phones per host, multiple hosts per phone

### NixOS Module
- **Service type**: systemd user service (like gpg-agent)
- **Config output**: All settings written to `~/.config/nix-key/config.json`
- **Device merge**: Nix-declared devices + runtime-paired devices from `~/.local/state/nix-key/devices.json`
- **Device cert override**: Each device in the Nix attrset can optionally specify `clientCert`/`clientKey` paths for programmatic mTLS cert definition
- **allowKeyListing**: Documented that phone can independently deny listing even if host allows

### OTEL / Tracing
- OpenTelemetry with W3C traceparent across mTLS boundary
- OTEL config transferred to phone during pairing if available
- Phone prompts user to accept OTEL separately from pairing
- Optional Jaeger service via NixOS module
- User has no OTEL experience — spec includes clear span hierarchy for implementation agents

### Failure Handling
- **Phone unreachable**: Fail fast, no retry. SSH clients handle retries.
- **User denies**: Immediate SSH_AGENT_FAILURE
- **Timeout**: Configurable `signTimeout` and `connectionTimeout`
- **IP change**: Cert fingerprint is identity, not IP. IP updated on next successful connection.

### Wire Protocol
- **gRPC with protobuf** over mTLS for host↔phone communication
- Rationale: automatic OTEL traceparent propagation via gRPC metadata, type-safe contracts, excellent Go + Kotlin support. 2-3MB APK overhead acceptable.
- Messages: ListKeys, Sign, Ping (and responses)

### Pairing Protocol (detailed)
- `nix-key pair` generates: host server cert (temp HTTPS), host client cert (normal operation)
- QR contains: host Tailscale IP, temp port, host server cert (full, not just fingerprint), one-time token, optional OTEL endpoint
- Phone POSTs: {phoneName, phoneTailscaleIp, phoneListenPort, phoneServerCert, oneTimeToken}
- Host holds HTTP connection until CLI user confirms
- Host responds: {hostName, hostClientCert, status: "approved"}
- Both sides store peer certs for pinning during normal operation

### Tailscale Authentication
- Phone uses `libtailscale` (userspace Go library, same as official Tailscale Android app)
- Requires Tailscale auth key or OAuth flow on first app launch
- Auth state persisted in encrypted storage
- NixOS module supports `tailscale.authKeyFile` for automated setups
- Integration tests use **headscale** (self-hosted control server) spun up in NixOS VM tests — fully self-contained, no external Tailscale account needed

### Android Keystore & Ed25519
- Android Keystore does NOT natively support Ed25519. Only ECDSA and RSA.
- Ed25519: software-generated via BouncyCastle, encrypted with Keystore-backed AES wrapping key
- ECDSA-P256: native Keystore hardware backing (TEE/StrongBox)
- Ed25519 is still default because it's the modern standard for SSH

### Host Secrets at Rest
- All mTLS private keys encrypted with age on host. Decrypted into memory at daemon startup.
- User mandated: "encrypt EVERYTHING sensitive" — no plaintext secrets on disk, file permissions are defense-in-depth only.

## Rejected Alternatives

- **RSA keys**: Rejected — legacy, larger, slower. Ed25519 and ECDSA-P256 cover all modern use cases.
- **Lock/Unlock agent**: Rejected — app foreground/background serves this role naturally.
- **Software-backed exportable keys**: Rejected by user ("rotate your shit"). YubiKey model only.
- **Persistent Tailscale connection**: Rejected — userspace Tailscale only when app is foregrounded. Battery and security benefit.
- **Docker for dev environment**: Rejected — Nix-first project, Nix handles everything.
- **System service**: Rejected — SSH agent is per-user, systemd user service is correct.
- **Diffie-Hellman key exchange for pairing**: Rejected in favor of simpler QR-based cert exchange. Physical proximity makes this acceptable.
- **JSON over raw TLS**: Rejected for wire protocol — manual OTEL propagation, no type safety. gRPC handles both automatically.
- **HTTP/2 with JSON**: Rejected — middle ground that doesn't excel. More complex than raw TCP but less feature-complete than gRPC.
- **Fingerprint-only in QR code (TOFU)**: Rejected — full cert in QR ensures no trust-on-first-use vulnerability.
- **Plaintext certs on disk with file permissions**: Rejected — user mandated encryption at rest for all secrets. Age encryption required.
- **Phone maintaining persistent connection to host**: Rejected — host connects to phone on demand. Phone is the server in normal operation.

## User Priorities
1. Security — mTLS, key pinning, non-extractable keys, physical proximity pairing
2. Ease of import — "super easy" Nix flake/channel import with `services.nix-key`
3. Testing — everything tested including NixOS VM tests
4. YubiKey UX — confirmation prompt on sign request, configurable per key
5. Simplicity — one way to do each thing, minimal config

## Non-Obvious Requirements
- Phone is the TLS **server** in normal operation (host connects to phone for signing)
- Phone is the TLS **client** during pairing (phone connects to host's temporary endpoint)
- OTEL config is transferred during pairing via QR code
- `allowKeyListing` has dual control — host config AND phone settings, phone can override
- Device `clientCert`/`clientKey` can be set declaratively in Nix OR by pairing
- Config file is designed to be easily populated for integration testing and debugging
