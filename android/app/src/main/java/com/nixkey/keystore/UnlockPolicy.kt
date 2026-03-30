package com.nixkey.keystore

/**
 * Per-key unlock policy controlling how a key is unlocked before it can sign.
 *
 * Unlock is a prerequisite step that keeps key material decrypted in memory.
 * Once unlocked, the signing policy (ConfirmationPolicy) determines per-sign
 * behavior. Keys are locked by default on app start (except NONE, which
 * eagerly unlocks).
 *
 * Default: PASSWORD
 */
enum class UnlockPolicy {
    /** No unlock required — key material is eagerly decrypted on app start. */
    NONE,

    /** Biometric only (fingerprint/face). */
    BIOMETRIC,

    /** Password/PIN/pattern only. */
    PASSWORD,

    /** Either biometric or password. */
    BIOMETRIC_PASSWORD,
}
