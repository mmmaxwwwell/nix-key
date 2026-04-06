package com.nixkey.ui.viewmodel

import androidx.lifecycle.ViewModel
import com.nixkey.data.SettingsRepository
import com.nixkey.keystore.ConfirmationPolicy
import com.nixkey.keystore.UnlockPolicy
import com.nixkey.tailscale.TailscaleManager
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

data class SettingsState(
    val allowKeyListing: Boolean = true,
    val defaultUnlockPolicy: UnlockPolicy = UnlockPolicy.PASSWORD,
    val defaultConfirmationPolicy: ConfirmationPolicy = ConfirmationPolicy.BIOMETRIC,
    val otelEnabled: Boolean = false,
    val otelEndpoint: String = "",
    val otelEndpointError: String? = null,
    val tailscaleIp: String = "",
    val tailnetName: String = ""
)

@HiltViewModel
class SettingsViewModel @Inject constructor(
    private val settingsRepository: SettingsRepository,
    private val tailscaleManager: TailscaleManager
) : ViewModel() {

    private val _state = MutableStateFlow(SettingsState())
    val state: StateFlow<SettingsState> = _state.asStateFlow()

    init {
        loadSettings()
    }

    private fun loadSettings() {
        val ip = tailscaleManager.getIp() ?: ""
        _state.value = SettingsState(
            allowKeyListing = settingsRepository.allowKeyListing,
            defaultUnlockPolicy = settingsRepository.defaultUnlockPolicy,
            defaultConfirmationPolicy = settingsRepository.defaultConfirmationPolicy,
            otelEnabled = settingsRepository.otelEnabled,
            otelEndpoint = settingsRepository.otelEndpoint,
            tailscaleIp = ip,
            tailnetName = if (ip.isNotEmpty()) "tailnet" else ""
        )
    }

    fun setAllowKeyListing(allow: Boolean) {
        settingsRepository.allowKeyListing = allow
        _state.update { it.copy(allowKeyListing = allow) }
    }

    fun setDefaultUnlockPolicy(policy: UnlockPolicy) {
        settingsRepository.defaultUnlockPolicy = policy
        _state.update { it.copy(defaultUnlockPolicy = policy) }
    }

    fun setDefaultConfirmationPolicy(policy: ConfirmationPolicy) {
        settingsRepository.defaultConfirmationPolicy = policy
        _state.update { it.copy(defaultConfirmationPolicy = policy) }
    }

    fun setOtelEnabled(enabled: Boolean) {
        settingsRepository.otelEnabled = enabled
        _state.update { it.copy(otelEnabled = enabled) }
    }

    fun setOtelEndpoint(endpoint: String) {
        settingsRepository.otelEndpoint = endpoint
        _state.update { it.copy(otelEndpoint = endpoint, otelEndpointError = null) }
    }

    fun validateOtelEndpoint() {
        val endpoint = _state.value.otelEndpoint
        if (endpoint.isNotEmpty() && !isValidHostPort(endpoint)) {
            _state.update { it.copy(otelEndpointError = "Invalid endpoint format (expected host:port)") }
        } else {
            _state.update { it.copy(otelEndpointError = null) }
        }
    }

    fun onReauthenticate(onNavigateToAuth: () -> Unit) {
        tailscaleManager.stop()
        tailscaleManager.clearAuthKey()
        onNavigateToAuth()
    }

    companion object {
        fun isValidHostPort(value: String): Boolean {
            val parts = value.split(":")
            if (parts.size != 2) return false
            val host = parts[0]
            val port = parts[1].toIntOrNull()
            return host.isNotEmpty() && port != null && port in 1..65535
        }
    }
}
