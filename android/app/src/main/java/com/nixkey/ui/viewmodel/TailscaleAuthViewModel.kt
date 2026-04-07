package com.nixkey.ui.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.nixkey.tailscale.TailscaleManager
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import timber.log.Timber

data class TailscaleAuthState(
    val phase: TailscaleAuthPhase = TailscaleAuthPhase.INPUT,
    val authKey: String = "",
    val error: String? = null,
    val oauthUrl: String? = null
)

enum class TailscaleAuthPhase {
    INPUT,
    CONNECTING,
    OAUTH_REQUIRED,
    SUCCESS,
    ERROR
}

@HiltViewModel
class TailscaleAuthViewModel @Inject constructor(
    private val tailscaleManager: TailscaleManager
) : ViewModel() {

    private val _state = MutableStateFlow(TailscaleAuthState())
    val state: StateFlow<TailscaleAuthState> = _state.asStateFlow()
    private var connectJob: Job? = null

    fun onAuthKeyChanged(key: String) {
        _state.update { it.copy(authKey = key) }
    }

    fun connectWithAuthKey() {
        val key = _state.value.authKey.trim()
        if (!isValidAuthKeyFormat(key)) {
            _state.update { it.copy(error = "Invalid auth key format") }
            return
        }

        _state.update {
            it.copy(
                phase = TailscaleAuthPhase.CONNECTING,
                error = null
            )
        }

        connectJob?.cancel()
        connectJob = viewModelScope.launch(Dispatchers.IO) {
            try {
                val timeoutJob = launch {
                    delay(CONNECTION_TIMEOUT_MS)
                    if (_state.value.phase == TailscaleAuthPhase.CONNECTING) {
                        Timber.w("Tailscale auth key connection timed out")
                        _state.update {
                            it.copy(
                                phase = TailscaleAuthPhase.ERROR,
                                error = "Connection timed out. Check your network and try again."
                            )
                        }
                    }
                }
                val oauthUrl = tailscaleManager.start(key)
                timeoutJob.cancel()
                if (oauthUrl != null) {
                    _state.update {
                        it.copy(
                            phase = TailscaleAuthPhase.OAUTH_REQUIRED,
                            oauthUrl = oauthUrl
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
                        error = userFriendlyError(e)
                    )
                }
            }
        }
    }

    fun connectWithOAuth() {
        _state.update {
            it.copy(
                phase = TailscaleAuthPhase.CONNECTING,
                error = null
            )
        }

        connectJob?.cancel()
        connectJob = viewModelScope.launch(Dispatchers.IO) {
            try {
                val timeoutJob = launch {
                    delay(CONNECTION_TIMEOUT_MS)
                    if (_state.value.phase == TailscaleAuthPhase.CONNECTING) {
                        Timber.w("Tailscale OAuth connection timed out")
                        _state.update {
                            it.copy(
                                phase = TailscaleAuthPhase.ERROR,
                                error = "Connection timed out. Check your network and try again."
                            )
                        }
                    }
                }
                val oauthUrl = tailscaleManager.start(null)
                timeoutJob.cancel()
                if (oauthUrl != null) {
                    _state.update {
                        it.copy(
                            phase = TailscaleAuthPhase.OAUTH_REQUIRED,
                            oauthUrl = oauthUrl
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
                        error = userFriendlyError(e)
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
        connectJob?.cancel()
        connectJob = null
        // Stop if still running from a failed attempt
        if (tailscaleManager.isRunning()) {
            tailscaleManager.stop()
        }
        _state.update {
            TailscaleAuthState(authKey = it.authKey)
        }
    }

    private fun userFriendlyError(e: Exception): String = when {
        e is IllegalStateException && e.message?.contains("already running") == true ->
            "Connection failed. Please try again."
        e.message?.contains("timeout", ignoreCase = true) == true ->
            "Connection timed out. Check your network and try again."
        else ->
            "Unable to connect to Tailscale. Please check your network and try again."
    }

    companion object {
        private const val CONNECTION_TIMEOUT_MS = 30_000L

        private val AUTH_KEY_PATTERN = "^tskey-(auth-)?[a-zA-Z0-9-]+$".toRegex()

        fun isValidAuthKeyFormat(key: String): Boolean {
            if (key.isEmpty() || key.contains("\\s".toRegex())) return false
            return AUTH_KEY_PATTERN.matches(key)
        }
    }
}
