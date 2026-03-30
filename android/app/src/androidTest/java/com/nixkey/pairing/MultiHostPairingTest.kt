package com.nixkey.pairing

import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.AndroidJUnit4
import com.nixkey.bridge.GoPhoneServer
import com.nixkey.data.HostRepository
import com.nixkey.data.PairedHost
import com.nixkey.keystore.ConfirmationPolicy
import com.nixkey.keystore.KeyManager
import com.nixkey.keystore.KeyType
import com.nixkey.keystore.KeyUnlockManager
import com.nixkey.keystore.SignRequestQueue
import com.nixkey.keystore.SignRequestStatus
import io.grpc.ManagedChannelBuilder
import java.util.concurrent.atomic.AtomicReference
import nixkey.v1.NixKey.ListKeysRequest
import nixkey.v1.NixKey.SignRequest as GrpcSignRequest
import nixkey.v1.NixKeyAgentGrpc
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Multi-host pairing integration test (T-AI-18, FR-030).
 *
 * Pairs a phone with two mock hosts, verifies both are stored in
 * EncryptedSharedPreferences, then verifies sign requests for keys
 * associated with each host work independently through the gRPC server.
 */
@RunWith(AndroidJUnit4::class)
class MultiHostPairingTest {

    private lateinit var hostRepository: HostRepository
    private lateinit var keyManager: KeyManager
    private lateinit var signRequestQueue: SignRequestQueue
    private lateinit var keyUnlockManager: KeyUnlockManager
    private lateinit var goPhoneServer: GoPhoneServer

    private val createdKeyAliases = mutableListOf<String>()

    @Before
    fun setUp() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        hostRepository = HostRepository(context)
        keyManager = KeyManager(context)
        signRequestQueue = SignRequestQueue()
        keyUnlockManager = KeyUnlockManager()
        goPhoneServer = GoPhoneServer(keyManager, signRequestQueue, keyUnlockManager)

        // Clear any existing hosts from previous test runs
        for (host in hostRepository.listHosts()) {
            hostRepository.removeHost(host.id)
        }
    }

    @After
    fun tearDown() {
        goPhoneServer.stop()
        for (host in hostRepository.listHosts()) {
            hostRepository.removeHost(host.id)
        }
        for (alias in createdKeyAliases) {
            try {
                keyManager.deleteKey(alias)
            } catch (_: Exception) {
                // ignore cleanup errors
            }
        }
    }

    @Test
    fun multiHostPairing_bothHostsStoredAndSignIndependently() {
        // --- Phase 1: Pair two hosts and verify storage ---

        val host1 = PairedHost(
            id = "host1_fp_multi",
            hostName = "nixos-desktop",
            tailscaleIp = "100.64.0.1",
            hostClientCertFingerprint = "host1_fp_multi",
            hostClientCert = "-----BEGIN CERTIFICATE-----\nHOST1CERT\n-----END CERTIFICATE-----",
            phoneServerCertAlias = "nixkey_phone_server_cert",
            otelEndpoint = null,
            otelEnabled = false,
            pairedAt = 1000L,
        )
        hostRepository.addHost(host1)

        val host2 = PairedHost(
            id = "host2_fp_multi",
            hostName = "nixos-laptop",
            tailscaleIp = "100.64.0.2",
            hostClientCertFingerprint = "host2_fp_multi",
            hostClientCert = "-----BEGIN CERTIFICATE-----\nHOST2CERT\n-----END CERTIFICATE-----",
            phoneServerCertAlias = "nixkey_phone_server_cert",
            otelEndpoint = "100.64.0.99:4317",
            otelEnabled = true,
            pairedAt = 2000L,
        )
        hostRepository.addHost(host2)

        // Verify both hosts are stored in EncryptedSharedPreferences
        val hosts = hostRepository.listHosts()
        assertEquals("Should have 2 paired hosts", 2, hosts.size)

        val storedHost1 = hostRepository.getHost("host1_fp_multi")
        assertNotNull("Host 1 should be retrievable", storedHost1)
        assertEquals("nixos-desktop", storedHost1!!.hostName)
        assertEquals("100.64.0.1", storedHost1.tailscaleIp)

        val storedHost2 = hostRepository.getHost("host2_fp_multi")
        assertNotNull("Host 2 should be retrievable", storedHost2)
        assertEquals("nixos-laptop", storedHost2!!.hostName)
        assertEquals("100.64.0.2", storedHost2.tailscaleIp)

        // --- Phase 2: Create one key per host and sign independently ---

        val key1 = keyManager.createKey(
            "desktop-key",
            KeyType.ECDSA_P256,
            signingPolicy = ConfirmationPolicy.ALWAYS_ASK,
        )
        createdKeyAliases.add(key1.alias)

        val key2 = keyManager.createKey(
            "laptop-key",
            KeyType.ECDSA_P256,
            signingPolicy = ConfirmationPolicy.ALWAYS_ASK,
        )
        createdKeyAliases.add(key2.alias)

        // Unlock both keys so ConfirmerAdapter doesn't flag needsUnlock
        keyUnlockManager.unlock(key1)
        keyUnlockManager.unlock(key2)

        goPhoneServer.start("127.0.0.1:0")
        Thread.sleep(500)
        assertTrue("Server should be running", goPhoneServer.isRunning())

        val channel = ManagedChannelBuilder
            .forAddress("127.0.0.1", goPhoneServer.port())
            .usePlaintext()
            .build()

        try {
            val stub = NixKeyAgentGrpc.newBlockingStub(channel)

            // Verify both keys are visible via gRPC
            val listResp = stub.listKeys(ListKeysRequest.getDefaultInstance())
            val fp1Found = listResp.keysList.any { it.fingerprint == key1.fingerprint }
            val fp2Found = listResp.keysList.any { it.fingerprint == key2.fingerprint }
            assertTrue("Key 1 (desktop) should be listed", fp1Found)
            assertTrue("Key 2 (laptop) should be listed", fp2Found)

            // Sign with key1 (host1's key) — auto-approve from background thread
            val host1Name = AtomicReference<String>()
            val sig1 = signWithApproval(stub, key1.fingerprint, "data-from-desktop", host1Name)
            assertTrue("Signature from key1 should not be empty", sig1.isNotEmpty())

            // Sign with key2 (host2's key) — auto-approve from background thread
            val host2Name = AtomicReference<String>()
            val sig2 = signWithApproval(stub, key2.fingerprint, "data-from-laptop", host2Name)
            assertTrue("Signature from key2 should not be empty", sig2.isNotEmpty())

            // Signatures for different data/keys should differ
            assertTrue(
                "Signatures from different keys/data should differ",
                !sig1.contentEquals(sig2),
            )

            // --- Phase 3: Verify hosts are still intact after signing ---
            val hostsAfter = hostRepository.listHosts()
            assertEquals("Both hosts should still be stored after signing", 2, hostsAfter.size)
            assertNotNull("Host 1 still present", hostRepository.getHost("host1_fp_multi"))
            assertNotNull("Host 2 still present", hostRepository.getHost("host2_fp_multi"))
        } finally {
            channel.shutdownNow()
        }
    }

    @Test
    fun multiHostPairing_removeOneHostDoesNotAffectOther() {
        val host1 = PairedHost(
            id = "host_remove_a",
            hostName = "host-alpha",
            tailscaleIp = "100.64.1.1",
            hostClientCertFingerprint = "fp_alpha",
            hostClientCert = "CERT_A",
        )
        val host2 = PairedHost(
            id = "host_remove_b",
            hostName = "host-beta",
            tailscaleIp = "100.64.1.2",
            hostClientCertFingerprint = "fp_beta",
            hostClientCert = "CERT_B",
        )
        hostRepository.addHost(host1)
        hostRepository.addHost(host2)
        assertEquals(2, hostRepository.listHosts().size)

        // Remove host1, verify host2 is unaffected
        hostRepository.removeHost("host_remove_a")
        assertEquals(1, hostRepository.listHosts().size)
        assertNotNull("Host 2 should survive host 1 removal", hostRepository.getHost("host_remove_b"))
        assertEquals(null, hostRepository.getHost("host_remove_a"))
    }

    /**
     * Sends a sign RPC and auto-approves from a background thread.
     * Returns the raw signature bytes.
     */
    private fun signWithApproval(
        stub: NixKeyAgentGrpc.NixKeyAgentBlockingStub,
        fingerprint: String,
        data: String,
        capturedHostName: AtomicReference<String>,
    ): ByteArray {
        val approveThread = Thread {
            waitForRequest()
            val current = signRequestQueue.currentRequest.value ?: return@Thread
            capturedHostName.set(current.hostName)
            signRequestQueue.complete(current.requestId, SignRequestStatus.APPROVED)
            goPhoneServer.confirmerAdapter.notifyCompletion(
                current.requestId,
                SignRequestStatus.APPROVED,
            )
        }
        approveThread.start()

        val req = GrpcSignRequest.newBuilder()
            .setKeyFingerprint(fingerprint)
            .setData(com.google.protobuf.ByteString.copyFrom(data.toByteArray()))
            .setFlags(0)
            .build()
        val resp = stub.sign(req)

        approveThread.join(5000)
        return resp.signature.toByteArray()
    }

    private fun waitForRequest() {
        var attempts = 0
        while (signRequestQueue.currentRequest.value == null && attempts < 50) {
            Thread.sleep(100)
            attempts++
        }
    }
}
