package com.nixkey.keystore

import java.time.Instant

data class SshKeyInfo(
    val alias: String,
    val displayName: String,
    val keyType: KeyType,
    val fingerprint: String,
    /** Per-key unlock policy (default: PASSWORD). Controls how the key is unlocked. */
    val unlockPolicy: UnlockPolicy = UnlockPolicy.PASSWORD,
    /** Per-key signing policy (default: BIOMETRIC). Controls per-sign confirmation. */
    val confirmationPolicy: ConfirmationPolicy = ConfirmationPolicy.BIOMETRIC,
    val createdAt: Instant,
    val wrappingKeyAlias: String?,
)
