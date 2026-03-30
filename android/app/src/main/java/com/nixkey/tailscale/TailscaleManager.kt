package com.nixkey.tailscale

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import timber.log.Timber
import java.io.File
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference
import javax.annotation.concurrent.ThreadSafe
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Tailnet connection state for UI display (FR-110).
 */
enum class TailnetConnectionState {
    CONNECTED,
    CONNECTING,
    DISCONNECTED,
}

/**
 * Manages the userspace Tailscale node lifecycle.
 *
 * Responsibilities:
 * - Initialize libtailscale with pre-authorized key or interactive OAuth
 * - Start/stop the Tailscale node with app foreground/background transitions
 * - Provide the current Tailscale IP address for binding the gRPC server
 * - Persist auth state in encrypted storage so re-auth is not needed on every open
 *
 * [FR-013]: Phone uses userspace Tailscale via libtailscale.
 * [FR-013a]: Auth key or OAuth flow, persisted in encrypted storage.
 */
@ThreadSafe
@Singleton
class TailscaleManager @Inject constructor(
    private val backend: TailscaleBackend,
    private val context: Context,
) {
    private val running = AtomicBoolean(false)
    private val tailscaleIp = AtomicReference<String?>(null)
    private val pendingOAuthUrl = AtomicReference<String?>(null)

    private val _connectionState = MutableStateFlow(TailnetConnectionState.DISCONNECTED)

    /** Reactive Tailnet connection state for UI indicators (FR-110). */
    val connectionState: StateFlow<TailnetConnectionState> = _connectionState.asStateFlow()

    private val encryptedPrefs by lazy {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        EncryptedSharedPreferences.create(
            context,
            PREFS_FILE,
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    }

    /**
     * Start the Tailscale node.
     *
     * If an auth key is provided (either as parameter or from encrypted settings),
     * it is used for automatic Tailnet join. Otherwise, an OAuth URL is returned
     * for interactive authentication.
     *
     * @param authKey Optional pre-authorized key. If null, checks encrypted storage.
     * @return An OAuth URL if interactive auth is needed, or null if auth completed.
     * @throws IllegalStateException if already running
     */
    fun start(authKey: String? = null): String? {
        if (running.getAndSet(true)) {
            throw IllegalStateException("TailscaleManager is already running")
        }

        val effectiveAuthKey = authKey ?: getStoredAuthKey()
        val dataDir = getTailscaleDataDir()

        Timber.i("TailscaleManager starting (authKey=%s)", if (effectiveAuthKey != null) "present" else "missing")
        _connectionState.value = TailnetConnectionState.CONNECTING

        val oauthUrl = try {
            backend.start(effectiveAuthKey, dataDir)
        } catch (e: Exception) {
            running.set(false)
            _connectionState.value = TailnetConnectionState.DISCONNECTED
            Timber.e(e, "TailscaleManager failed to start")
            throw e
        }

        if (oauthUrl != null) {
            pendingOAuthUrl.set(oauthUrl)
            // Stay in CONNECTING until OAuth completes
            Timber.i("TailscaleManager requires OAuth authentication")
        } else {
            refreshIp()
            _connectionState.value = if (tailscaleIp.get() != null) {
                TailnetConnectionState.CONNECTED
            } else {
                TailnetConnectionState.CONNECTING
            }
            if (effectiveAuthKey != null) {
                storeAuthKey(effectiveAuthKey)
            }
            Timber.i("TailscaleManager started, ip=%s", tailscaleIp.get())
        }

        return oauthUrl
    }

    /**
     * Stop the Tailscale node. Safe to call even if not running.
     */
    fun stop() {
        if (!running.getAndSet(false)) {
            return
        }
        Timber.i("TailscaleManager stopping")
        tailscaleIp.set(null)
        pendingOAuthUrl.set(null)
        _connectionState.value = TailnetConnectionState.DISCONNECTED
        backend.stop()
        Timber.i("TailscaleManager stopped")
    }

    /**
     * Returns true if the Tailscale node is currently running.
     */
    fun isRunning(): Boolean = running.get() && backend.isRunning()

    /**
     * Returns the current Tailscale IP address, or null if not connected.
     */
    fun getIp(): String? {
        if (!running.get()) return null
        // Refresh from backend in case it changed
        refreshIp()
        val ip = tailscaleIp.get()
        _connectionState.value = if (ip != null) {
            TailnetConnectionState.CONNECTED
        } else if (running.get()) {
            TailnetConnectionState.CONNECTING
        } else {
            TailnetConnectionState.DISCONNECTED
        }
        return ip
    }

    /**
     * Returns a pending OAuth URL if interactive auth is required, or null.
     */
    fun getPendingOAuthUrl(): String? = pendingOAuthUrl.get()

    /**
     * Called after OAuth flow completes successfully.
     * Clears the pending OAuth URL and refreshes the IP.
     */
    fun onOAuthComplete() {
        pendingOAuthUrl.set(null)
        refreshIp()
        _connectionState.value = if (tailscaleIp.get() != null) {
            TailnetConnectionState.CONNECTED
        } else {
            TailnetConnectionState.CONNECTING
        }
        Timber.i("TailscaleManager OAuth complete, ip=%s", tailscaleIp.get())
    }

    /**
     * Store an auth key in encrypted preferences for future use.
     */
    fun storeAuthKey(key: String) {
        encryptedPrefs.edit().putString(KEY_AUTH_KEY, key).apply()
    }

    /**
     * Clear the stored auth key (e.g., on logout).
     */
    fun clearAuthKey() {
        encryptedPrefs.edit().remove(KEY_AUTH_KEY).apply()
    }

    /**
     * Returns true if an auth key is stored in encrypted preferences.
     */
    fun hasStoredAuthKey(): Boolean {
        return getStoredAuthKey() != null
    }

    private fun getStoredAuthKey(): String? {
        return encryptedPrefs.getString(KEY_AUTH_KEY, null)
    }

    private fun refreshIp() {
        tailscaleIp.set(backend.getIp())
    }

    private fun getTailscaleDataDir(): String {
        val dir = File(context.filesDir, "tailscale")
        if (!dir.exists()) {
            dir.mkdirs()
        }
        return dir.absolutePath
    }

    /**
     * Handle stale authentication (FR-115).
     *
     * When the Tailscale node is running but cannot obtain an IP
     * (e.g., auth key expired or revoked), this method stops the node,
     * clears stored credentials, and transitions to DISCONNECTED so
     * the UI can present a re-auth flow instead of crashing.
     *
     * @return true if stale auth was detected and handled
     */
    fun handleStaleAuth(): Boolean {
        if (!running.get()) return false
        val ip = backend.getIp()
        if (ip != null) return false

        Timber.w("TailscaleManager: stale auth detected, triggering re-auth flow")
        stop()
        clearAuthKey()
        return true
    }

    companion object {
        private const val PREFS_FILE = "nixkey_tailscale"
        private const val KEY_AUTH_KEY = "tailscale_auth_key"
    }
}
