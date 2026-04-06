package com.nixkey.ui.viewmodel

import androidx.lifecycle.ViewModel
import com.nixkey.data.SettingsRepository
import com.nixkey.keystore.ConfirmationPolicy
import com.nixkey.keystore.UnlockPolicy
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
    val otelEndpoint: String = ""
)

@HiltViewModel
class SettingsViewModel @Inject constructor(
    private val settingsRepository: SettingsRepository
) : ViewModel() {

    private val _state = MutableStateFlow(SettingsState())
    val state: StateFlow<SettingsState> = _state.asStateFlow()

    init {
        loadSettings()
    }

    private fun loadSettings() {
        _state.value = SettingsState(
            allowKeyListing = settingsRepository.allowKeyListing,
            defaultUnlockPolicy = settingsRepository.defaultUnlockPolicy,
            defaultConfirmationPolicy = settingsRepository.defaultConfirmationPolicy,
            otelEnabled = settingsRepository.otelEnabled,
            otelEndpoint = settingsRepository.otelEndpoint
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
        _state.update { it.copy(otelEndpoint = endpoint) }
    }
}
