package com.nixkey.data

import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.AndroidJUnit4
import com.nixkey.bridge.GoPhoneServer
import com.nixkey.keystore.ConfirmationPolicy
import com.nixkey.keystore.KeyManager
import com.nixkey.keystore.KeyType
import com.nixkey.keystore.KeyUnlockManager
import com.nixkey.keystore.SignRequestQueue
import com.nixkey.keystore.SignRequestStatus
import io.grpc.ManagedChannelBuilder
import io.grpc.StatusRuntimeException
import nixkey.v1.NixKey.ListKeysRequest
import nixkey.v1.NixKeyAgentGrpc
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * T-AI-16: Expired cert mid-session behavior (FR-E18).
 *
 * Tests how the system handles hosts with expired or invalid certificates.
 * Since actual mTLS cert validation is handled at the Go gRPC layer,
 * these tests verify the HostRepository and sign flow behavior when a
 * host's cert data changes or becomes invalid mid-session.
 */
@RunWith(AndroidJUnit4::class)
class ExpiredCertTest {

    private lateinit var hostRepository: HostRepository
    private lateinit var keyManager: KeyManager
    private lateinit var signRequestQueue: SignRequestQueue
    private lateinit var keyUnlockManager: KeyUnlockManager
    private lateinit var goPhoneServer: GoPhoneServer
    private val createdAliases = mutableListOf<String>()

    @Before
    fun setUp() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        hostRepository = HostRepository(context)
        keyManager = KeyManager(context)
        signRequestQueue = SignRequestQueue()
        keyUnlockManager = KeyUnlockManager()
        goPhoneServer = GoPhoneServer(keyManager, signRequestQueue, keyUnlockManager)

        // Clean up hosts from prior runs
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
        for (alias in createdAliases) {
            try {
                keyManager.deleteKey(alias)
            } catch (_: Exception) {
                // ignore
            }
        }
    }

    @Test
    fun hostWithExpiredCert_canBeRemovedAndReAdded() {
        // Simulate a host whose cert has expired — store it, then remove and re-pair
        val host = PairedHost(
            id = "expired_host",
            hostName = "expired-desktop",
            tailscaleIp = "100.64.0.10",
            hostClientCertFingerprint = "old_expired_fp",
            hostClientCert = "-----BEGIN CERTIFICATE-----\nEXPIRED_CERT\n-----END CERTIFICATE-----",
            pairedAt = 1000L,
        )
        hostRepository.addHost(host)
        assertNotNull(hostRepository.getHost("expired_host"))

        // Remove the expired host
        hostRepository.removeHost("expired_host")
        assertNull("Expired host should be removed", hostRepository.getHost("expired_host"))

        // Re-add with fresh cert (simulating re-pairing)
        val newHost = host.copy(
            hostClientCertFingerprint = "new_valid_fp",
            hostClientCert = "-----BEGIN CERTIFICATE-----\nNEW_VALID_CERT\n-----END CERTIFICATE-----",
            pairedAt = System.currentTimeMillis(),
        )
        hostRepository.addHost(newHost)

        val retrieved = hostRepository.getHost("expired_host")
        assertNotNull("Re-paired host should exist", retrieved)
        assertEquals("new_valid_fp", retrieved!!.hostClientCertFingerprint)
    }

    @Test
    fun hostCertUpdate_doesNotAffectOtherHosts() {
        val host1 = PairedHost(
            id = "cert_host1",
            hostName = "host-one",
            tailscaleIp = "100.64.0.11",
            hostClientCertFingerprint = "fp1",
            hostClientCert = "CERT1",
        )
        val host2 = PairedHost(
            id = "cert_host2",
            hostName = "host-two",
            tailscaleIp = "100.64.0.12",
            hostClientCertFingerprint = "fp2",
            hostClientCert = "CERT2",
        )
        hostRepository.addHost(host1)
        hostRepository.addHost(host2)

        // Remove and re-add host1 with new cert
        hostRepository.removeHost("cert_host1")
        hostRepository.addHost(
            host1.copy(
                hostClientCertFingerprint = "fp1_new",
                hostClientCert = "CERT1_NEW",
            ),
        )

        // host2 should be unaffected
        val h2 = hostRepository.getHost("cert_host2")
        assertNotNull(h2)
        assertEquals("fp2", h2!!.hostClientCertFingerprint)
        assertEquals("CERT2", h2.hostClientCert)
    }

    @Test
    fun signStillWorksAfterHostCertRotation() {
        // Pair a host, create+unlock key, start server, sign, then rotate host cert,
        // verify signing still works (server doesn't use host cert data at runtime)
        val host = PairedHost(
            id = "rotate_host",
            hostName = "rotate-desktop",
            tailscaleIp = "100.64.0.13",
            hostClientCertFingerprint = "original_fp",
            hostClientCert = "ORIGINAL_CERT",
        )
        hostRepository.addHost(host)

        val key = keyManager.createKey(
            "rotate-key",
            KeyType.ECDSA_P256,
            signingPolicy = ConfirmationPolicy.ALWAYS_ASK,
        )
        createdAliases.add(key.alias)
        keyUnlockManager.unlock(key)

        goPhoneServer.start("127.0.0.1:0")
        waitForServerReady()
        assertTrue("Server should be running", goPhoneServer.isRunning())

        val channel = ManagedChannelBuilder
            .forAddress("127.0.0.1", goPhoneServer.port())
            .usePlaintext()
            .build()

        try {
            val stub = NixKeyAgentGrpc.newBlockingStub(channel)

            // Verify keys are listed
            val resp = stub.listKeys(ListKeysRequest.getDefaultInstance())
            assertTrue(resp.keysList.any { it.fingerprint == key.fingerprint })

            // Sign before cert rotation
            val sig1 = signWithApproval(stub, key.fingerprint, "before-rotation")
            assertTrue("Signature should not be empty", sig1.isNotEmpty())

            // Rotate host cert in repository (simulating cert refresh)
            hostRepository.removeHost("rotate_host")
            hostRepository.addHost(
                host.copy(
                    hostClientCertFingerprint = "rotated_fp",
                    hostClientCert = "ROTATED_CERT",
                ),
            )

            // Sign after cert rotation — server is still running, sign should work
            val sig2 = signWithApproval(stub, key.fingerprint, "after-rotation")
            assertTrue("Signature after rotation should not be empty", sig2.isNotEmpty())
        } finally {
            channel.shutdownNow()
        }
    }

    @Test
    fun deniedSignRequest_returnsPermissionDenied() {
        val key = keyManager.createKey(
            "deny-key",
            KeyType.ECDSA_P256,
            signingPolicy = ConfirmationPolicy.ALWAYS_ASK,
        )
        createdAliases.add(key.alias)
        keyUnlockManager.unlock(key)

        goPhoneServer.start("127.0.0.1:0")
        waitForServerReady()

        val channel = ManagedChannelBuilder
            .forAddress("127.0.0.1", goPhoneServer.port())
            .usePlaintext()
            .build()

        try {
            val stub = NixKeyAgentGrpc.newBlockingStub(channel)

            // Deny from background thread
            val denyThread = Thread {
                waitForRequest()
                val current = signRequestQueue.currentRequest.value ?: return@Thread
                signRequestQueue.complete(current.requestId, SignRequestStatus.DENIED)
                goPhoneServer.confirmerAdapter.notifyCompletion(
                    current.requestId,
                    SignRequestStatus.DENIED,
                )
            }
            denyThread.start()

            try {
                val req = nixkey.v1.NixKey.SignRequest.newBuilder()
                    .setKeyFingerprint(key.fingerprint)
                    .setData(com.google.protobuf.ByteString.copyFrom("deny-data".toByteArray()))
                    .setFlags(0)
                    .build()
                stub.sign(req)
                fail("Should have thrown PERMISSION_DENIED")
            } catch (e: StatusRuntimeException) {
                assertEquals(io.grpc.Status.Code.PERMISSION_DENIED, e.status.code)
            }

            denyThread.join(5000)
        } finally {
            channel.shutdownNow()
        }
    }

    private fun signWithApproval(
        stub: NixKeyAgentGrpc.NixKeyAgentBlockingStub,
        fingerprint: String,
        data: String,
    ): ByteArray {
        val approveThread = Thread {
            waitForRequest()
            val current = signRequestQueue.currentRequest.value ?: return@Thread
            signRequestQueue.complete(current.requestId, SignRequestStatus.APPROVED)
            goPhoneServer.confirmerAdapter.notifyCompletion(
                current.requestId,
                SignRequestStatus.APPROVED,
            )
        }
        approveThread.start()

        val req = nixkey.v1.NixKey.SignRequest.newBuilder()
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

    private fun waitForServerReady(timeoutMs: Long = 10_000) {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline) {
            if (goPhoneServer.port() > 0) return
            Thread.sleep(200)
        }
        assertTrue("Server should have a port within ${timeoutMs}ms", goPhoneServer.port() > 0)
    }
}
