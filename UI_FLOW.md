# nix-key UI Flow

## Host CLI

The host CLI (`nix-key`) is a cobra-based command-line tool. There is no GUI on the host side.

### First-Run Flow

```
1. User runs `nix-key pair`
   → Starts ephemeral HTTPS server on local Tailscale interface
   → Generates QR code containing: host IP, port, fingerprint
   → Displays QR code in terminal
   → Waits for phone to connect

2. Phone scans QR code (see Android flow below)
   → mTLS certificate exchange completes
   → Device registered in devices.json (certs age-encrypted)
   → QR server shuts down

3. User runs `nix-key daemon` (or systemd starts it)
   → SSH agent socket created at SSH_AUTH_SOCK
   → Control socket created for CLI ↔ daemon IPC
   → Daemon loads device registry and waits for sign requests
```

### Day-to-Day Flow

```
1. SSH client connects to SSH_AUTH_SOCK
   → daemon receives sign request via SSH agent protocol
   → daemon dials phone over gRPC/mTLS via Tailscale
   → phone prompts user for biometric confirmation
   → phone signs with hardware keystore, returns signature
   → daemon returns signature to SSH client

2. Management commands (while daemon is running):
   nix-key devices        → list paired phones
   nix-key status         → show daemon health + connected devices
   nix-key export <key>   → print SSH public key to stdout
   nix-key test <device>  → ping a paired phone
   nix-key revoke <device>→ remove a paired phone
   nix-key logs           → tail daemon logs (human-readable)
   nix-key config         → show current configuration
```

## Android App

### Navigation Graph

```
┌─────────────────┐
│ TailscaleAuth    │ ← start (if Tailscale not authenticated)
│ Screen           │
└────────┬────────┘
         │ onAuthSuccess
         ▼
┌─────────────────┐
│ ServerList       │ ← start (if Tailscale already authenticated)
│ Screen           │
├─────────────────┤
│ • Lists paired   │
│   hosts          │──► KeyList (per host)
│ • "Scan QR" FAB │──► Pairing Screen
│ • Settings gear │──► Settings Screen
└─────────────────┘

┌─────────────────┐
│ Pairing Screen   │
│                  │
│ • QR scanner     │
│ • Shows pairing  │
│   progress       │
│ • On success →   │
│   back to        │
│   ServerList     │
└─────────────────┘

┌─────────────────┐
│ KeyList Screen   │
│ (per host)       │
│                  │
│ • Lists SSH keys │
│   for this host  │
│ • "Create Key"  │──► KeyDetail (new)
│ • Tap key       │──► KeyDetail (existing)
└─────────────────┘

┌─────────────────┐
│ KeyDetail Screen │
│                  │
│ • Key info:      │
│   algorithm,     │
│   fingerprint,   │
│   created date   │
│ • New key:       │
│   choose algo,   │
│   create in HW   │
│   keystore       │
└─────────────────┘

┌─────────────────┐
│ Settings Screen  │
│                  │
│ • Confirmation   │
│   policy (always │
│   / first-time / │
│   never)         │
│ • Tailscale      │
│   status         │
└─────────────────┘
```

### Sign Request Dialog

When the host sends a sign request via gRPC, a `SignRequestDialog` overlay appears regardless of current screen:

```
┌──────────────────────┐
│ Sign Request          │
│                       │
│ Host: <hostname>      │
│ Key: <fingerprint>    │
│                       │
│ [Biometric Prompt]    │
│                       │
│ [Approve] [Deny]      │
└──────────────────────┘
```

The biometric prompt uses Android's `BiometricPrompt` API, gated by the user's confirmation policy setting.

### Deep Link Support

The app supports deep links with a pairing payload. When a QR code contains a URL, the app navigates directly to the Pairing Screen with the payload pre-filled.

### Background Service

`GrpcServerService` runs as a foreground service (persistent notification). It keeps the gRPC server alive so the host can reach the phone for sign requests even when the app is not in the foreground.
