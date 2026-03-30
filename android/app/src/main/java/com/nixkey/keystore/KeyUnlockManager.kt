package com.nixkey.keystore

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import timber.log.Timber
import java.util.concurrent.ConcurrentHashMap
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Runtime state for an unlocked key. Key material is held in memory while unlocked
 * and wiped on lock or process kill.
 */
data class UnlockedKeyState(
    val alias: String,
    val fingerprint: String,
)

/**
 * Manages the runtime lock/unlock state of SSH keys.
 *
 * Keys start locked on app start. Unlocking keeps the key available for signing
 * without re-prompting the unlock policy. The signing policy (ConfirmationPolicy)
 * is evaluated separately for each sign request.
 *
 * Key material persists in memory across background/foreground transitions but is
 * wiped on process kill (no persistence of unlock state).
 *
 * Keys with [UnlockPolicy.NONE] are eagerly unlocked on app start via [eagerUnlockNoneKeys].
 */
@Singleton
class KeyUnlockManager @Inject constructor() {

    private val unlockedKeys = ConcurrentHashMap<String, UnlockedKeyState>()

    private val _unlockedFingerprints = MutableStateFlow<Set<String>>(emptySet())

    /** Set of fingerprints for currently unlocked keys. Observable by UI. */
    val unlockedFingerprints: StateFlow<Set<String>> = _unlockedFingerprints.asStateFlow()

    /**
     * Mark a key as unlocked. Called after successful unlock authentication.
     */
    fun unlock(keyInfo: SshKeyInfo) {
        val state = UnlockedKeyState(
            alias = keyInfo.alias,
            fingerprint = keyInfo.fingerprint,
        )
        unlockedKeys[keyInfo.fingerprint] = state
        publishState()
        Timber.i("Key unlocked: alias=%s fingerprint=%s", keyInfo.alias, keyInfo.fingerprint)
    }

    /**
     * Lock a key, wiping its unlocked state from memory.
     */
    fun lock(fingerprint: String) {
        val removed = unlockedKeys.remove(fingerprint)
        if (removed != null) {
            publishState()
            Timber.i("Key locked: alias=%s fingerprint=%s", removed.alias, fingerprint)
        }
    }

    /**
     * Returns true if the key with the given fingerprint is currently unlocked.
     */
    fun isUnlocked(fingerprint: String): Boolean = unlockedKeys.containsKey(fingerprint)

    /**
     * Lock all keys. Called on manual "lock all" or when clearing state.
     */
    fun lockAll() {
        unlockedKeys.clear()
        publishState()
        Timber.i("All keys locked")
    }

    /**
     * Eagerly unlock all keys that have [UnlockPolicy.NONE].
     * Called on app start.
     */
    fun eagerUnlockNoneKeys(keys: List<SshKeyInfo>) {
        for (key in keys) {
            if (key.unlockPolicy == UnlockPolicy.NONE) {
                unlock(key)
                Timber.i("Eager unlock (NONE policy): alias=%s", key.alias)
            }
        }
    }

    private fun publishState() {
        _unlockedFingerprints.value = unlockedKeys.keys.toSet()
    }
}
