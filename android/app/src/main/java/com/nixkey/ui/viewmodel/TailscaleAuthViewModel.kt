package com.nixkey.ui.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.nixkey.tailscale.TailscaleManager
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import timber.log.Timber
import javax.inject.Inject

data class TailscaleAuthState(
    val phase: TailscaleAuthPhase = TailscaleAuthPhase.INPUT,
    val authKey: String = "",
    val error: String? = null,
    val oauthUrl: String? = null,
)

enum class TailscaleAuthPhase {
    INPUT,
    CONNECTING,
    OAUTH_REQUIRED,
    SUCCESS,
    ERROR,
}

@HiltViewModel
class TailscaleAuthViewModel @Inject constructor(
    private val tailscaleManager: TailscaleManager,
) : ViewModel() {

    private val _state = MutableStateFlow(TailscaleAuthState())
    val state: StateFlow<TailscaleAuthState> = _state.asStateFlow()

    fun onAuthKeyChanged(key: String) {
        _state.update { it.copy(authKey = key) }
    }

    fun connectWithAuthKey() {
        val key = _state.value.authKey.trim()
        if (key.isEmpty()) {
            _state.update { it.copy(error = "Auth key cannot be empty") }
            return
        }

        _state.update {
            it.copy(
                phase = TailscaleAuthPhase.CONNECTING,
                error = null,
            )
        }

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val oauthUrl = tailscaleManager.start(key)
                if (oauthUrl != null) {
                    _state.update {
                        it.copy(
                            phase = TailscaleAuthPhase.OAUTH_REQUIRED,
                            oauthUrl = oauthUrl,
                        )
                    }
                } else {
                    Timber.i("Tailscale connected via auth key")
                    _state.update { it.copy(phase = TailscaleAuthPhase.SUCCESS) }
                }
            } catch (e: Exception) {
                Timber.e(e, "Tailscale auth failed")
                _state.update {
                    it.copy(
                        phase = TailscaleAuthPhase.ERROR,
                        error = "Connection failed: ${e.message}",
                    )
                }
            }
        }
    }

    fun connectWithOAuth() {
        _state.update {
            it.copy(
                phase = TailscaleAuthPhase.CONNECTING,
                error = null,
            )
        }

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val oauthUrl = tailscaleManager.start(null)
                if (oauthUrl != null) {
                    _state.update {
                        it.copy(
                            phase = TailscaleAuthPhase.OAUTH_REQUIRED,
                            oauthUrl = oauthUrl,
                        )
                    }
                } else {
                    Timber.i("Tailscale connected via OAuth")
                    _state.update { it.copy(phase = TailscaleAuthPhase.SUCCESS) }
                }
            } catch (e: Exception) {
                Timber.e(e, "Tailscale OAuth failed")
                _state.update {
                    it.copy(
                        phase = TailscaleAuthPhase.ERROR,
                        error = "Connection failed: ${e.message}",
                    )
                }
            }
        }
    }

    fun onOAuthComplete() {
        tailscaleManager.onOAuthComplete()
        Timber.i("Tailscale OAuth complete")
        _state.update { it.copy(phase = TailscaleAuthPhase.SUCCESS, oauthUrl = null) }
    }

    fun retry() {
        // Stop if still running from a failed attempt
        if (tailscaleManager.isRunning()) {
            tailscaleManager.stop()
        }
        _state.update {
            TailscaleAuthState(authKey = it.authKey)
        }
    }
}
