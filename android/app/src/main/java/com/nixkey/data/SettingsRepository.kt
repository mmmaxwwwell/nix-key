package com.nixkey.data

import android.content.Context
import android.content.SharedPreferences
import com.nixkey.keystore.ConfirmationPolicy
import com.nixkey.keystore.UnlockPolicy
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class SettingsRepository @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    private val prefs: SharedPreferences by lazy {
        context.getSharedPreferences(PREFS_FILE, Context.MODE_PRIVATE)
    }

    var allowKeyListing: Boolean
        get() = prefs.getBoolean(KEY_ALLOW_KEY_LISTING, true)
        set(value) = prefs.edit().putBoolean(KEY_ALLOW_KEY_LISTING, value).apply()

    var defaultUnlockPolicy: UnlockPolicy
        get() {
            val name = prefs.getString(KEY_DEFAULT_UNLOCK_POLICY, UnlockPolicy.PASSWORD.name)
            return try {
                UnlockPolicy.valueOf(name ?: UnlockPolicy.PASSWORD.name)
            } catch (_: IllegalArgumentException) {
                UnlockPolicy.PASSWORD
            }
        }
        set(value) = prefs.edit().putString(KEY_DEFAULT_UNLOCK_POLICY, value.name).apply()

    var defaultConfirmationPolicy: ConfirmationPolicy
        get() {
            val name = prefs.getString(KEY_DEFAULT_POLICY, ConfirmationPolicy.BIOMETRIC.name)
            return try {
                ConfirmationPolicy.valueOf(name ?: ConfirmationPolicy.BIOMETRIC.name)
            } catch (_: IllegalArgumentException) {
                ConfirmationPolicy.BIOMETRIC
            }
        }
        set(value) = prefs.edit().putString(KEY_DEFAULT_POLICY, value.name).apply()

    var otelEnabled: Boolean
        get() = prefs.getBoolean(KEY_OTEL_ENABLED, false)
        set(value) = prefs.edit().putBoolean(KEY_OTEL_ENABLED, value).apply()

    var otelEndpoint: String
        get() = prefs.getString(KEY_OTEL_ENDPOINT, "") ?: ""
        set(value) = prefs.edit().putString(KEY_OTEL_ENDPOINT, value).apply()

    var listenPort: Int
        get() = prefs.getInt(KEY_LISTEN_PORT, DEFAULT_LISTEN_PORT)
        set(value) = prefs.edit().putInt(KEY_LISTEN_PORT, value).apply()

    companion object {
        const val DEFAULT_LISTEN_PORT = 29418
        private const val PREFS_FILE = "nixkey_settings"
        private const val KEY_ALLOW_KEY_LISTING = "allow_key_listing"
        private const val KEY_DEFAULT_UNLOCK_POLICY = "default_unlock_policy"
        private const val KEY_DEFAULT_POLICY = "default_confirmation_policy"
        private const val KEY_OTEL_ENABLED = "otel_enabled"
        private const val KEY_OTEL_ENDPOINT = "otel_endpoint"
        private const val KEY_LISTEN_PORT = "listen_port"
    }
}
