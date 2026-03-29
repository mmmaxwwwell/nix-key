package com.nixkey.keystore

import java.time.Instant

data class SshKeyInfo(
    val alias: String,
    val displayName: String,
    val keyType: KeyType,
    val fingerprint: String,
    val confirmationPolicy: ConfirmationPolicy,
    val createdAt: Instant,
    val wrappingKeyAlias: String?,
)
