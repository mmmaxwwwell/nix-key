package com.nixkey.data

data class PairedHost(
    val id: String,
    val name: String,
    val tailscaleIp: String,
    val status: ConnectionStatus = ConnectionStatus.UNKNOWN,
)

enum class ConnectionStatus {
    REACHABLE,
    UNREACHABLE,
    UNKNOWN,
}
