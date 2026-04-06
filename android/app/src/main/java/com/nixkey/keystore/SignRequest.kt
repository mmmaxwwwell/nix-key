package com.nixkey.keystore

import java.security.MessageDigest
import java.time.Instant
import java.util.UUID

/**
 * Status of a sign request in the queue.
 */
enum class SignRequestStatus {
    PENDING,
    APPROVED,
    DENIED,
    TIMEOUT
}

/**
 * Represents a pending SSH sign request from a paired host.
 *
 * @param requestId Unique identifier for this request
 * @param keyFingerprint SHA256 fingerprint of the requested key
 * @param hostName Display name of the host that sent the request
 * @param keyName Display name of the key being requested
 * @param dataToSign Raw data to be signed
 * @param confirmationPolicy The key's confirmation policy
 * @param receivedAt When the request was received
 * @param status Current status of the request
 */
data class SignRequest(
    val requestId: String = UUID.randomUUID().toString(),
    val keyFingerprint: String,
    val hostName: String,
    val keyName: String,
    val dataToSign: ByteArray,
    val unlockPolicy: UnlockPolicy = UnlockPolicy.PASSWORD,
    val confirmationPolicy: ConfirmationPolicy = ConfirmationPolicy.BIOMETRIC,
    /** Whether this request needs unlock before signing can proceed. */
    val needsUnlock: Boolean = false,
    val receivedAt: Instant = Instant.now(),
    val status: SignRequestStatus = SignRequestStatus.PENDING
) {
    /**
     * Returns a truncated SHA-256 hash of the data to sign, for display purposes.
     * Shows the first 16 hex characters (8 bytes) followed by "...".
     */
    fun dataHashTruncated(): String {
        val digest = MessageDigest.getInstance("SHA-256")
        val hash = digest.digest(dataToSign)
        val hex = hash.joinToString("") { "%02x".format(it) }
        return "${hex.take(16)}..."
    }

    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (other !is SignRequest) return false
        return requestId == other.requestId
    }

    override fun hashCode(): Int = requestId.hashCode()
}
