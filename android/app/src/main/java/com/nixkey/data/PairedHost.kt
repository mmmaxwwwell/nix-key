package com.nixkey.data

data class PairedHost(
    val id: String,
    val hostName: String,
    val tailscaleIp: String,
    val hostClientCertFingerprint: String = "",
    val hostClientCert: String = "",
    val phoneServerCertAlias: String = "",
    val otelEndpoint: String? = null,
    val otelEnabled: Boolean = false,
    val pairedAt: Long = System.currentTimeMillis(),
    val status: ConnectionStatus = ConnectionStatus.UNKNOWN
)

enum class ConnectionStatus {
    REACHABLE,
    UNREACHABLE,
    UNKNOWN
}
