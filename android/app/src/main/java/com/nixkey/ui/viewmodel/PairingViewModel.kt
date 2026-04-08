package com.nixkey.ui.viewmodel

import android.util.Base64
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.nixkey.data.HostRepository
import com.nixkey.data.PairedHost
import com.nixkey.data.SettingsRepository
import com.nixkey.logging.SecurityLog
import com.nixkey.pairing.PairingClient
import com.nixkey.pairing.QrPayload
import com.nixkey.tailscale.TailscaleManager
import dagger.hilt.android.lifecycle.HiltViewModel
import java.security.MessageDigest
import javax.inject.Inject
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import org.json.JSONObject
import timber.log.Timber

data class PairingState(
    val phase: PairingPhase = PairingPhase.SCANNING,
    val payload: QrPayload? = null,
    val showOtelPrompt: Boolean = false,
    val otelAccepted: Boolean = false,
    val error: String? = null,
    val isPairing: Boolean = false,
    val pairingStatusText: String = ""
)

enum class PairingPhase {
    SCANNING,
    CONFIRM_HOST,
    CONFIRM_OTEL,
    PAIRING,
    SUCCESS,
    ERROR
}

@HiltViewModel
class PairingViewModel @Inject constructor(
    private val hostRepository: HostRepository,
    private val settingsRepository: SettingsRepository,
    private val tailscaleManager: TailscaleManager,
    private val pairingClient: PairingClient
) : ViewModel() {

    private val _state = MutableStateFlow(PairingState())
    val state: StateFlow<PairingState> = _state.asStateFlow()

    fun onQrScanned(rawValue: String) {
        try {
            val payload = decodeQrPayload(rawValue)
            SecurityLog.pairingAttempt(payload.host, payload.host)
            _state.update {
                it.copy(
                    phase = PairingPhase.CONFIRM_HOST,
                    payload = payload,
                    error = null
                )
            }
        } catch (e: Exception) {
            Timber.e(e, "Failed to decode QR payload")
            _state.update {
                it.copy(
                    phase = PairingPhase.ERROR,
                    error = "Not a nix-key pairing code"
                )
            }
        }
    }

    fun onHostConfirmed() {
        val payload = _state.value.payload ?: return
        if (!payload.otel.isNullOrEmpty()) {
            _state.update { it.copy(phase = PairingPhase.CONFIRM_OTEL) }
        } else {
            startPairing(otelAccepted = false)
        }
    }

    fun onHostDenied() {
        val payload = _state.value.payload
        if (payload != null) {
            SecurityLog.pairingDenied(payload.host)
        }
        _state.update { PairingState() }
    }

    fun onOtelAccepted() {
        startPairing(otelAccepted = true)
    }

    fun onOtelDenied() {
        startPairing(otelAccepted = false)
    }

    fun resetState() {
        _state.update { PairingState() }
    }

    private fun startPairing(otelAccepted: Boolean) {
        val payload = _state.value.payload ?: return
        _state.update {
            it.copy(
                phase = PairingPhase.PAIRING,
                isPairing = true,
                otelAccepted = otelAccepted,
                pairingStatusText = "Connecting to host..."
            )
        }

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val phoneName = android.os.Build.MODEL
                val tsIp = tailscaleManager.getIp() ?: run {
                    Timber.w("Tailscale IP unavailable, using payload host as fallback for pairing")
                    payload.host
                }
                val listenPort = settingsRepository.listenPort

                // Determine phone server cert alias: reuse if one exists, else use default
                val serverCertAlias = getOrCreateServerCertAlias()

                _state.update { it.copy(pairingStatusText = "Waiting for host approval...") }

                // POST to host pairing endpoint
                val response = pairingClient.pair(
                    host = payload.host,
                    port = payload.port,
                    serverCertPem = payload.cert,
                    token = payload.token,
                    phoneName = phoneName,
                    tailscaleIp = tsIp,
                    listenPort = listenPort,
                    phoneServerCert = serverCertAlias
                )

                // Compute host ID from the host client cert fingerprint
                val hostId = computeCertFingerprint(response.hostClientCert)

                // Store host in EncryptedSharedPreferences (FR-030: multiple hosts)
                val pairedHost = PairedHost(
                    id = hostId,
                    hostName = response.hostName,
                    tailscaleIp = payload.host,
                    hostClientCertFingerprint = hostId,
                    hostClientCert = response.hostClientCert,
                    phoneServerCertAlias = serverCertAlias,
                    otelEndpoint = if (otelAccepted) payload.otel else null,
                    otelEnabled = otelAccepted,
                    pairedAt = System.currentTimeMillis()
                )
                hostRepository.addHost(pairedHost)

                // Update OTEL settings if accepted
                if (otelAccepted && !payload.otel.isNullOrEmpty()) {
                    settingsRepository.otelEnabled = true
                    settingsRepository.otelEndpoint = payload.otel
                }

                SecurityLog.pairingSuccess(response.hostName)

                _state.update {
                    it.copy(
                        phase = PairingPhase.SUCCESS,
                        isPairing = false
                    )
                }
            } catch (e: Exception) {
                Timber.e(e, "Pairing failed")
                SecurityLog.pairingFailed(payload.host, e.message ?: "unknown")
                _state.update {
                    it.copy(
                        phase = PairingPhase.ERROR,
                        isPairing = false,
                        error = "Pairing failed: ${e.message}"
                    )
                }
            }
        }
    }

    private fun getOrCreateServerCertAlias(): String {
        // Reuse the existing server cert alias if any host is already paired
        val existingHosts = hostRepository.listHosts()
        if (existingHosts.isNotEmpty()) {
            val alias = existingHosts.first().phoneServerCertAlias
            if (alias.isNotEmpty()) return alias
        }
        // Default alias for phone server cert (generated by GrpcServerService/KeyManager)
        return PHONE_SERVER_CERT_ALIAS
    }

    companion object {
        const val PHONE_SERVER_CERT_ALIAS = "nixkey_phone_server_cert"

        fun decodeQrPayload(rawValue: String): QrPayload {
            val jsonStr = String(Base64.decode(rawValue, Base64.DEFAULT))
            val json = JSONObject(jsonStr)
            require(json.getInt("v") == 1) { "Unsupported QR payload version" }
            return QrPayload(
                v = json.getInt("v"),
                host = json.getString("host"),
                port = json.getInt("port"),
                cert = json.getString("cert"),
                token = json.getString("token"),
                otel = json.optString("otel", null)
            )
        }

        fun computeCertFingerprint(certPem: String): String {
            val digest = MessageDigest.getInstance("SHA-256")
            val hash = digest.digest(certPem.toByteArray())
            return hash.joinToString("") { "%02x".format(it) }
        }
    }
}
