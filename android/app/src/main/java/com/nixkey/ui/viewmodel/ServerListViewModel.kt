package com.nixkey.ui.viewmodel

import androidx.lifecycle.ViewModel
import com.nixkey.data.HostRepository
import com.nixkey.data.PairedHost
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import javax.inject.Inject

@HiltViewModel
class ServerListViewModel @Inject constructor(
    private val hostRepository: HostRepository,
) : ViewModel() {

    private val _hosts = MutableStateFlow<List<PairedHost>>(emptyList())
    val hosts: StateFlow<List<PairedHost>> = _hosts.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        _hosts.value = hostRepository.listHosts()
    }
}
