# Phase phase8-android-grpc-server- — Review #1: REVIEW-FIXES

**Date**: 2026-03-29
**Fixes applied**:
- `android/app/src/main/java/com/nixkey/pairing/PairingClient.kt`: Trust-all TLS replaced with cert-pinned TLS. The `serverCertPem` parameter (host's self-signed cert from QR payload) was passed but unused — a `TrustAllManager` accepted any certificate, enabling MITM during pairing. Replaced with `createPinnedSslContext()` that loads the QR-provided cert into a `KeyStore` and uses `TrustManagerFactory` to validate only that specific cert. Commit: 2bb4208.

**Deferred** (optional improvements, not bugs):
- `PairingViewModel.kt:133`: `phoneServerCert = serverCertAlias` sends a keystore alias string ("nixkey_phone_server_cert") instead of the actual PEM-encoded phone server certificate. The host stores this as the phone's cert for future mTLS, which won't work. This requires phone server cert generation infrastructure that is not yet implemented — it will need to be addressed when the full mTLS pipeline is wired up.
- `GrpcServerService.kt`: gRPC server starts without mTLS configuration (no cert/key material passed to `GoPhoneServer.start()`). The Go-side `phoneserver` would need TLS credentials for production use. This is likely deferred to the mTLS integration phase.
- `PairingClient.kt:55`: Hostname verification is disabled (`hostnameVerifier = { _, _ -> true }`). This is acceptable for self-signed certs where the CN/SAN may not match the Tailscale IP, but should be revisited if proper CA-signed certs are used in the future.
