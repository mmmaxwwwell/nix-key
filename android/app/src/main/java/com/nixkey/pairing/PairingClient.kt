package com.nixkey.pairing

import java.io.BufferedReader
import java.io.ByteArrayInputStream
import java.io.InputStreamReader
import java.io.OutputStreamWriter
import java.net.URL
import java.security.KeyStore
import java.security.SecureRandom
import java.security.cert.CertificateFactory
import javax.inject.Inject
import javax.inject.Singleton
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManagerFactory
import org.json.JSONObject
import timber.log.Timber

data class PairingResponse(
    val hostName: String,
    val hostClientCert: String,
    val status: String
)

@Singleton
class PairingClient @Inject constructor() {

    fun pair(
        host: String,
        port: Int,
        serverCertPem: String,
        token: String,
        phoneName: String,
        tailscaleIp: String,
        listenPort: Int,
        phoneServerCert: String
    ): PairingResponse {
        val url = URL("https://$host:$port/pair")

        val requestBody = JSONObject().apply {
            put("phoneName", phoneName)
            put("tailscaleIp", tailscaleIp)
            put("listenPort", listenPort)
            put("serverCert", phoneServerCert)
            put("token", token)
        }

        Timber.d("Pairing POST to %s", url)

        // Pin to the host's self-signed cert from the QR payload
        val sslContext = createPinnedSslContext(serverCertPem)

        val connection = url.openConnection() as HttpsURLConnection
        connection.sslSocketFactory = sslContext.socketFactory
        connection.hostnameVerifier = javax.net.ssl.HostnameVerifier { _, _ -> true }
        connection.requestMethod = "POST"
        connection.setRequestProperty("Content-Type", "application/json")
        connection.doOutput = true
        connection.connectTimeout = 30_000
        connection.readTimeout = 120_000 // Host waits for user confirmation

        try {
            OutputStreamWriter(connection.outputStream).use { writer ->
                writer.write(requestBody.toString())
                writer.flush()
            }

            val responseCode = connection.responseCode
            val responseBody = if (responseCode in 200..299) {
                BufferedReader(InputStreamReader(connection.inputStream)).use { it.readText() }
            } else {
                val errorBody = connection.errorStream?.let {
                    BufferedReader(InputStreamReader(it)).use { reader -> reader.readText() }
                } ?: ""
                throw PairingException("Host returned status $responseCode: $errorBody")
            }

            val json = JSONObject(responseBody)
            val status = json.getString("status")

            if (status != "approved") {
                throw PairingException("Pairing was denied by host")
            }

            return PairingResponse(
                hostName = json.getString("hostName"),
                hostClientCert = json.getString("hostClientCert"),
                status = status
            )
        } finally {
            connection.disconnect()
        }
    }

    private fun createPinnedSslContext(certPem: String): SSLContext {
        val cf = CertificateFactory.getInstance("X.509")
        val cert = cf.generateCertificate(ByteArrayInputStream(certPem.toByteArray()))
        val ks = KeyStore.getInstance(KeyStore.getDefaultType()).apply {
            load(null, null)
            setCertificateEntry("host-pairing", cert)
        }
        val tmf = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply {
            init(ks)
        }
        return SSLContext.getInstance("TLS").apply {
            init(null, tmf.trustManagers, SecureRandom())
        }
    }
}

class PairingException(message: String) : Exception(message)
