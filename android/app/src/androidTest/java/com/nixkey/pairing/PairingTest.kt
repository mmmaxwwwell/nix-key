package com.nixkey.pairing

import android.util.Base64
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.AndroidJUnit4
import com.nixkey.data.HostRepository
import com.nixkey.ui.viewmodel.PairingViewModel
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Instrumented test for the pairing flow (FR-022, FR-023, FR-024, FR-026, FR-030).
 *
 * Tests that:
 * 1. QR payload is decoded correctly
 * 2. Two mock hosts can be paired and stored in EncryptedSharedPreferences
 * 3. Both hosts are retrievable after pairing
 * 4. OTEL configuration is stored when accepted
 */
@RunWith(AndroidJUnit4::class)
class PairingTest {

    private lateinit var hostRepository: HostRepository

    @Before
    fun setUp() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        hostRepository = HostRepository(context)
        // Clear any existing hosts from previous test runs
        for (host in hostRepository.listHosts()) {
            hostRepository.removeHost(host.id)
        }
    }

    @After
    fun tearDown() {
        // Clean up stored hosts
        for (host in hostRepository.listHosts()) {
            hostRepository.removeHost(host.id)
        }
    }

    @Test
    fun decodeQrPayload_validPayload_parsesCorrectly() {
        val payload = createQrPayload(
            host = "100.64.0.1",
            port = 12345,
            cert = "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----",
            token = "abc123",
            otel = null,
        )

        val decoded = PairingViewModel.decodeQrPayload(payload)

        assertEquals(1, decoded.v)
        assertEquals("100.64.0.1", decoded.host)
        assertEquals(12345, decoded.port)
        assertEquals("-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----", decoded.cert)
        assertEquals("abc123", decoded.token)
        assertEquals(null, decoded.otel)
    }

    @Test
    fun decodeQrPayload_withOtel_parsesCorrectly() {
        val payload = createQrPayload(
            host = "100.64.0.1",
            port = 12345,
            cert = "CERT",
            token = "token",
            otel = "100.64.0.1:4317",
        )

        val decoded = PairingViewModel.decodeQrPayload(payload)

        assertEquals("100.64.0.1:4317", decoded.otel)
    }

    @Test(expected = IllegalArgumentException::class)
    fun decodeQrPayload_wrongVersion_throwsException() {
        val json = JSONObject().apply {
            put("v", 2)
            put("host", "100.64.0.1")
            put("port", 12345)
            put("cert", "CERT")
            put("token", "token")
        }
        val encoded = Base64.encodeToString(json.toString().toByteArray(), Base64.DEFAULT)
        PairingViewModel.decodeQrPayload(encoded)
    }

    @Test
    fun pairTwoHosts_bothStoredCorrectly() {
        // Simulate pairing with host 1
        val host1 = com.nixkey.data.PairedHost(
            id = "host1_fingerprint_abc",
            hostName = "nixos-desktop",
            tailscaleIp = "100.64.0.1",
            hostClientCertFingerprint = "host1_fingerprint_abc",
            hostClientCert = "-----BEGIN CERTIFICATE-----\nHOST1CERT\n-----END CERTIFICATE-----",
            phoneServerCertAlias = "nixkey_phone_server_cert",
            otelEndpoint = null,
            otelEnabled = false,
            pairedAt = 1000L,
        )
        hostRepository.addHost(host1)

        // Simulate pairing with host 2 (with OTEL)
        val host2 = com.nixkey.data.PairedHost(
            id = "host2_fingerprint_def",
            hostName = "nixos-laptop",
            tailscaleIp = "100.64.0.2",
            hostClientCertFingerprint = "host2_fingerprint_def",
            hostClientCert = "-----BEGIN CERTIFICATE-----\nHOST2CERT\n-----END CERTIFICATE-----",
            phoneServerCertAlias = "nixkey_phone_server_cert",
            otelEndpoint = "100.64.0.99:4317",
            otelEnabled = true,
            pairedAt = 2000L,
        )
        hostRepository.addHost(host2)

        // Verify both hosts are stored (FR-030: multiple paired hosts)
        val hosts = hostRepository.listHosts()
        assertEquals(2, hosts.size)

        // Find each host by ID
        val storedHost1 = hosts.find { it.id == "host1_fingerprint_abc" }
        assertNotNull("Host 1 should be stored", storedHost1)
        assertEquals("nixos-desktop", storedHost1!!.hostName)
        assertEquals("100.64.0.1", storedHost1.tailscaleIp)
        assertEquals("host1_fingerprint_abc", storedHost1.hostClientCertFingerprint)
        assertEquals("-----BEGIN CERTIFICATE-----\nHOST1CERT\n-----END CERTIFICATE-----", storedHost1.hostClientCert)
        assertEquals("nixkey_phone_server_cert", storedHost1.phoneServerCertAlias)
        assertEquals(null, storedHost1.otelEndpoint)
        assertFalse(storedHost1.otelEnabled)
        assertEquals(1000L, storedHost1.pairedAt)

        val storedHost2 = hosts.find { it.id == "host2_fingerprint_def" }
        assertNotNull("Host 2 should be stored", storedHost2)
        assertEquals("nixos-laptop", storedHost2!!.hostName)
        assertEquals("100.64.0.2", storedHost2.tailscaleIp)
        assertEquals("host2_fingerprint_def", storedHost2.hostClientCertFingerprint)
        assertEquals("-----BEGIN CERTIFICATE-----\nHOST2CERT\n-----END CERTIFICATE-----", storedHost2.hostClientCert)
        assertEquals("nixkey_phone_server_cert", storedHost2.phoneServerCertAlias)
        assertEquals("100.64.0.99:4317", storedHost2.otelEndpoint)
        assertTrue(storedHost2.otelEnabled)
        assertEquals(2000L, storedHost2.pairedAt)
    }

    @Test
    fun getHost_returnsCorrectHost() {
        val host = com.nixkey.data.PairedHost(
            id = "test_host_id",
            hostName = "test-host",
            tailscaleIp = "100.64.0.5",
            hostClientCertFingerprint = "fp_test",
            hostClientCert = "CERT_DATA",
            phoneServerCertAlias = "alias_test",
            otelEndpoint = "endpoint",
            otelEnabled = true,
            pairedAt = 3000L,
        )
        hostRepository.addHost(host)

        val retrieved = hostRepository.getHost("test_host_id")
        assertNotNull(retrieved)
        assertEquals("test-host", retrieved!!.hostName)
        assertEquals("100.64.0.5", retrieved.tailscaleIp)
        assertEquals("fp_test", retrieved.hostClientCertFingerprint)
        assertEquals("CERT_DATA", retrieved.hostClientCert)
        assertEquals("alias_test", retrieved.phoneServerCertAlias)
        assertEquals("endpoint", retrieved.otelEndpoint)
        assertTrue(retrieved.otelEnabled)
        assertEquals(3000L, retrieved.pairedAt)
    }

    @Test
    fun getHost_nonexistentId_returnsNull() {
        val result = hostRepository.getHost("nonexistent")
        assertEquals(null, result)
    }

    @Test
    fun removeHost_removesAllData() {
        val host = com.nixkey.data.PairedHost(
            id = "remove_test_id",
            hostName = "to-remove",
            tailscaleIp = "100.64.0.9",
            hostClientCertFingerprint = "fp_remove",
            hostClientCert = "CERT_REMOVE",
        )
        hostRepository.addHost(host)
        assertEquals(1, hostRepository.listHosts().size)

        hostRepository.removeHost("remove_test_id")
        assertEquals(0, hostRepository.listHosts().size)
        assertEquals(null, hostRepository.getHost("remove_test_id"))
    }

    @Test
    fun computeCertFingerprint_producesConsistentHash() {
        val certPem = "-----BEGIN CERTIFICATE-----\nTESTDATA\n-----END CERTIFICATE-----"
        val fp1 = PairingViewModel.computeCertFingerprint(certPem)
        val fp2 = PairingViewModel.computeCertFingerprint(certPem)
        assertEquals(fp1, fp2)
        assertEquals(64, fp1.length) // SHA-256 hex = 64 chars
    }

    private fun createQrPayload(
        host: String,
        port: Int,
        cert: String,
        token: String,
        otel: String?,
    ): String {
        val json = JSONObject().apply {
            put("v", 1)
            put("host", host)
            put("port", port)
            put("cert", cert)
            put("token", token)
            if (otel != null) put("otel", otel)
        }
        return Base64.encodeToString(json.toString().toByteArray(), Base64.DEFAULT)
    }
}
