package com.nixkey.logging

import timber.log.Timber

/**
 * Structured security event logger. All security events are logged at INFO
 * or above as required by FR-093/FR-094. Events include pairing attempts,
 * sign requests, and mTLS failures.
 */
object SecurityLog {
    private const val TAG = "security"

    fun pairingAttempt(hostName: String, hostIp: String) {
        Timber.tag(TAG).i("pairing_attempt host=%s ip=%s", hostName, hostIp)
    }

    fun pairingSuccess(hostName: String) {
        Timber.tag(TAG).i("pairing_success host=%s", hostName)
    }

    fun pairingDenied(hostName: String) {
        Timber.tag(TAG).w("pairing_denied host=%s", hostName)
    }

    fun pairingFailed(hostName: String, reason: String) {
        Timber.tag(TAG).e("pairing_failed host=%s reason=%s", hostName, reason)
    }

    fun signRequest(hostName: String, keyFingerprint: String) {
        Timber.tag(TAG).i("sign_request host=%s key=%s", hostName, keyFingerprint)
    }

    fun signApproved(hostName: String, keyFingerprint: String) {
        Timber.tag(TAG).i("sign_approved host=%s key=%s", hostName, keyFingerprint)
    }

    fun signDenied(hostName: String, keyFingerprint: String) {
        Timber.tag(TAG).w("sign_denied host=%s key=%s", hostName, keyFingerprint)
    }

    fun signFailed(hostName: String, keyFingerprint: String, reason: String) {
        Timber.tag(TAG).e("sign_failed host=%s key=%s reason=%s", hostName, keyFingerprint, reason)
    }

    fun mtlsFailure(peerIp: String, reason: String) {
        Timber.tag(TAG).e("mtls_failure peer=%s reason=%s", peerIp, reason)
    }

    fun mtlsCertRejected(peerIp: String, fingerprint: String) {
        Timber.tag(TAG).e("mtls_cert_rejected peer=%s fingerprint=%s", peerIp, fingerprint)
    }
}
