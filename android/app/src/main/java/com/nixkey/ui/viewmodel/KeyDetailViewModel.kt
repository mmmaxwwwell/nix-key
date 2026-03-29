package com.nixkey.ui.viewmodel

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import com.nixkey.keystore.ConfirmationPolicy
import com.nixkey.keystore.KeyManager
import com.nixkey.keystore.KeyType
import com.nixkey.keystore.SshKeyInfo
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import javax.inject.Inject

data class KeyDetailState(
    val isCreateMode: Boolean = true,
    val keyInfo: SshKeyInfo? = null,
    val displayName: String = "",
    val keyType: KeyType = KeyType.ED25519,
    val confirmationPolicy: ConfirmationPolicy = ConfirmationPolicy.ALWAYS_ASK,
    val publicKeyString: String = "",
    val hasUnsavedChanges: Boolean = false,
    val error: String? = null,
    val showAutoApproveWarning: Boolean = false,
    val keyCreated: Boolean = false,
    val keyDeleted: Boolean = false,
)

@HiltViewModel
class KeyDetailViewModel @Inject constructor(
    private val keyManager: KeyManager,
    savedStateHandle: SavedStateHandle,
) : ViewModel() {

    private val keyId: String? = savedStateHandle.get<String>("keyId")
    val isCreateMode: Boolean = keyId == null || keyId == "new"

    private val _state = MutableStateFlow(KeyDetailState(isCreateMode = isCreateMode))
    val state: StateFlow<KeyDetailState> = _state.asStateFlow()

    init {
        if (!isCreateMode && keyId != null) {
            loadKey(keyId)
        }
    }

    private fun loadKey(alias: String) {
        val info = keyManager.getKey(alias)
        if (info != null) {
            val pubKey = try {
                keyManager.exportPublicKey(alias)
            } catch (_: Exception) {
                ""
            }
            _state.update {
                it.copy(
                    isCreateMode = false,
                    keyInfo = info,
                    displayName = info.displayName,
                    keyType = info.keyType,
                    confirmationPolicy = info.confirmationPolicy,
                    publicKeyString = pubKey,
                )
            }
        }
    }

    fun setDisplayName(name: String) {
        _state.update {
            it.copy(
                displayName = name,
                hasUnsavedChanges = !it.isCreateMode &&
                    (name != it.keyInfo?.displayName || it.confirmationPolicy != it.keyInfo.confirmationPolicy),
            )
        }
    }

    fun setKeyType(type: KeyType) {
        if (_state.value.isCreateMode) {
            _state.update { it.copy(keyType = type) }
        }
    }

    fun setConfirmationPolicy(policy: ConfirmationPolicy) {
        if (policy == ConfirmationPolicy.AUTO_APPROVE) {
            _state.update { it.copy(showAutoApproveWarning = true) }
            return
        }
        applyConfirmationPolicy(policy)
    }

    fun confirmAutoApprove() {
        applyConfirmationPolicy(ConfirmationPolicy.AUTO_APPROVE)
        _state.update { it.copy(showAutoApproveWarning = false) }
    }

    fun dismissAutoApproveWarning() {
        _state.update { it.copy(showAutoApproveWarning = false) }
    }

    private fun applyConfirmationPolicy(policy: ConfirmationPolicy) {
        _state.update {
            it.copy(
                confirmationPolicy = policy,
                hasUnsavedChanges = !it.isCreateMode &&
                    (it.displayName != it.keyInfo?.displayName || policy != it.keyInfo.confirmationPolicy),
            )
        }
    }

    fun createKey() {
        val s = _state.value
        if (s.displayName.isBlank()) {
            _state.update { it.copy(error = "Name is required") }
            return
        }
        if (!s.displayName.matches(KEY_NAME_REGEX)) {
            _state.update { it.copy(error = "Name must be 1-64 characters (letters, numbers, hyphens, underscores)") }
            return
        }
        try {
            val info = keyManager.createKey(s.displayName, s.keyType, s.confirmationPolicy)
            val pubKey = keyManager.exportPublicKey(info.alias)
            _state.update {
                it.copy(
                    isCreateMode = false,
                    keyInfo = info,
                    publicKeyString = pubKey,
                    error = null,
                    keyCreated = true,
                )
            }
        } catch (e: Exception) {
            _state.update { it.copy(error = "Failed to create key: ${e.message}") }
        }
    }

    fun saveChanges() {
        val s = _state.value
        val alias = s.keyInfo?.alias ?: return
        if (s.displayName.isBlank()) {
            _state.update { it.copy(error = "Name is required") }
            return
        }
        if (!s.displayName.matches(KEY_NAME_REGEX)) {
            _state.update { it.copy(error = "Name must be 1-64 characters (letters, numbers, hyphens, underscores)") }
            return
        }
        try {
            keyManager.updateKey(alias, s.displayName, s.confirmationPolicy)
            val updatedInfo = keyManager.getKey(alias)
            _state.update {
                it.copy(
                    keyInfo = updatedInfo,
                    hasUnsavedChanges = false,
                    error = null,
                )
            }
        } catch (e: Exception) {
            _state.update { it.copy(error = "Failed to save: ${e.message}") }
        }
    }

    fun deleteKey() {
        val alias = _state.value.keyInfo?.alias ?: return
        try {
            keyManager.deleteKey(alias)
            _state.update { it.copy(keyDeleted = true) }
        } catch (e: Exception) {
            _state.update { it.copy(error = "Failed to delete: ${e.message}") }
        }
    }

    fun clearError() {
        _state.update { it.copy(error = null) }
    }

    companion object {
        private val KEY_NAME_REGEX = Regex("^[a-zA-Z0-9_-]{1,64}$")
    }
}
