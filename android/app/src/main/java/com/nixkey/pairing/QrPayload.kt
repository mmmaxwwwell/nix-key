package com.nixkey.pairing

data class QrPayload(
    val v: Int,
    val host: String,
    val port: Int,
    val cert: String,
    val token: String,
    val otel: String? = null
)
