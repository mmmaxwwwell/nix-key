package com.nixkey.ui.viewmodel

import androidx.lifecycle.ViewModel
import com.nixkey.keystore.KeyManager
import com.nixkey.keystore.SshKeyInfo
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import javax.inject.Inject

@HiltViewModel
class KeyListViewModel @Inject constructor(
    private val keyManager: KeyManager,
) : ViewModel() {

    private val _keys = MutableStateFlow<List<SshKeyInfo>>(emptyList())
    val keys: StateFlow<List<SshKeyInfo>> = _keys.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        _keys.value = keyManager.listKeys()
    }
}
