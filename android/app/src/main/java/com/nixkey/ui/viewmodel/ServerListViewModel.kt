package com.nixkey.ui.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.nixkey.data.HostRepository
import com.nixkey.data.PairedHost
import com.nixkey.tailscale.TailnetConnectionState
import com.nixkey.tailscale.TailscaleManager
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import timber.log.Timber

@HiltViewModel
class ServerListViewModel @Inject constructor(
    private val hostRepository: HostRepository,
    private val tailscaleManager: TailscaleManager
) : ViewModel() {

    private val _hosts = MutableStateFlow<List<PairedHost>>(emptyList())
    val hosts: StateFlow<List<PairedHost>> = _hosts.asStateFlow()

    private val _connectionError = MutableStateFlow<String?>(null)
    val connectionError: StateFlow<String?> = _connectionError.asStateFlow()

    private var timeoutJob: Job? = null

    init {
        refresh()
        viewModelScope.launch {
            tailscaleManager.connectionState.collect { state ->
                when (state) {
                    TailnetConnectionState.CONNECTING -> {
                        timeoutJob?.cancel()
                        timeoutJob = launch {
                            delay(CONNECTION_TIMEOUT_MS)
                            if (tailscaleManager.connectionState.value == TailnetConnectionState.CONNECTING) {
                                Timber.w("ServerList: connection timed out after %dms", CONNECTION_TIMEOUT_MS)
                                withContext(Dispatchers.IO) {
                                    tailscaleManager.stop()
                                }
                                _connectionError.value = "Connection timed out. Check your network and try again."
                            }
                        }
                    }
                    TailnetConnectionState.CONNECTED -> {
                        timeoutJob?.cancel()
                        _connectionError.value = null
                    }
                    TailnetConnectionState.DISCONNECTED -> {
                        timeoutJob?.cancel()
                    }
                }
            }
        }
    }

    fun refresh() {
        _hosts.value = hostRepository.listHosts()
    }

    fun retryConnection() {
        _connectionError.value = null
        viewModelScope.launch(Dispatchers.IO) {
            try {
                tailscaleManager.start()
            } catch (e: Exception) {
                Timber.e(e, "ServerList: retry connection failed")
                _connectionError.value = "Unable to connect. Please try again."
            }
        }
    }

    companion object {
        private const val CONNECTION_TIMEOUT_MS = 30_000L
    }
}
