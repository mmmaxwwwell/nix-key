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
            val hostName = prefs.getString("${id}_hostName", null) ?: return@mapNotNull null
            val ip = prefs.getString("${id}_ip", null) ?: return@mapNotNull null
            PairedHost(
                id = id,
                hostName = hostName,
                tailscaleIp = ip,
                hostClientCertFingerprint = prefs.getString("${id}_certFp", null) ?: "",
                hostClientCert = prefs.getString("${id}_clientCert", null) ?: "",
                phoneServerCertAlias = prefs.getString("${id}_serverCertAlias", null) ?: "",
                otelEndpoint = prefs.getString("${id}_otelEndpoint", null),
                otelEnabled = prefs.getBoolean("${id}_otelEnabled", false),
                pairedAt = prefs.getLong("${id}_pairedAt", 0L),
                status = ConnectionStatus.UNKNOWN,
            )
        }
    }

    fun getHost(hostId: String): PairedHost? {
        val ids = prefs.getStringSet(KEY_HOST_IDS, emptySet()) ?: emptySet()
        if (hostId !in ids) return null
        val hostName = prefs.getString("${hostId}_hostName", null) ?: return null
        val ip = prefs.getString("${hostId}_ip", null) ?: return null
        return PairedHost(
            id = hostId,
            hostName = hostName,
            tailscaleIp = ip,
            hostClientCertFingerprint = prefs.getString("${hostId}_certFp", null) ?: "",
            hostClientCert = prefs.getString("${hostId}_clientCert", null) ?: "",
            phoneServerCertAlias = prefs.getString("${hostId}_serverCertAlias", null) ?: "",
            otelEndpoint = prefs.getString("${hostId}_otelEndpoint", null),
            otelEnabled = prefs.getBoolean("${hostId}_otelEnabled", false),
            pairedAt = prefs.getLong("${hostId}_pairedAt", 0L),
            status = ConnectionStatus.UNKNOWN,
        )
    }

    fun addHost(host: PairedHost) {
        val ids = prefs.getStringSet(KEY_HOST_IDS, mutableSetOf())?.toMutableSet()
            ?: mutableSetOf()
        ids.add(host.id)
        prefs.edit()
            .putStringSet(KEY_HOST_IDS, ids)
            .putString("${host.id}_hostName", host.hostName)
            .putString("${host.id}_ip", host.tailscaleIp)
            .putString("${host.id}_certFp", host.hostClientCertFingerprint)
            .putString("${host.id}_clientCert", host.hostClientCert)
            .putString("${host.id}_serverCertAlias", host.phoneServerCertAlias)
            .putString("${host.id}_otelEndpoint", host.otelEndpoint)
            .putBoolean("${host.id}_otelEnabled", host.otelEnabled)
            .putLong("${host.id}_pairedAt", host.pairedAt)
            .apply()
    }

    fun removeHost(hostId: String) {
        val ids = prefs.getStringSet(KEY_HOST_IDS, mutableSetOf())?.toMutableSet()
            ?: mutableSetOf()
        ids.remove(hostId)
        prefs.edit()
            .putStringSet(KEY_HOST_IDS, ids)
            .remove("${hostId}_hostName")
            .remove("${hostId}_ip")
            .remove("${hostId}_certFp")
            .remove("${hostId}_clientCert")
            .remove("${hostId}_serverCertAlias")
            .remove("${hostId}_otelEndpoint")
            .remove("${hostId}_otelEnabled")
            .remove("${hostId}_pairedAt")
            .apply()
    }

    companion object {
        private const val PREFS_FILE = "nixkey_hosts"
        private const val KEY_HOST_IDS = "host_ids"
    }
}
