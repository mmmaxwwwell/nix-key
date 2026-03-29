package com.nixkey.data

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class HostRepository @Inject constructor(
    @dagger.hilt.android.qualifiers.ApplicationContext private val context: Context,
) {
    private val prefs by lazy {
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

    fun listHosts(): List<PairedHost> {
        val ids = prefs.getStringSet(KEY_HOST_IDS, emptySet()) ?: emptySet()
        return ids.mapNotNull { id ->
            val name = prefs.getString("${id}_name", null) ?: return@mapNotNull null
            val ip = prefs.getString("${id}_ip", null) ?: return@mapNotNull null
            PairedHost(
                id = id,
                name = name,
                tailscaleIp = ip,
                status = ConnectionStatus.UNKNOWN,
            )
        }
    }

    fun addHost(host: PairedHost) {
        val ids = prefs.getStringSet(KEY_HOST_IDS, mutableSetOf())?.toMutableSet()
            ?: mutableSetOf()
        ids.add(host.id)
        prefs.edit()
            .putStringSet(KEY_HOST_IDS, ids)
            .putString("${host.id}_name", host.name)
            .putString("${host.id}_ip", host.tailscaleIp)
            .apply()
    }

    fun removeHost(hostId: String) {
        val ids = prefs.getStringSet(KEY_HOST_IDS, mutableSetOf())?.toMutableSet()
            ?: mutableSetOf()
        ids.remove(hostId)
        prefs.edit()
            .putStringSet(KEY_HOST_IDS, ids)
            .remove("${hostId}_name")
            .remove("${hostId}_ip")
            .apply()
    }

    companion object {
        private const val PREFS_FILE = "nixkey_hosts"
        private const val KEY_HOST_IDS = "host_ids"
    }
}
