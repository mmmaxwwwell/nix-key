package com.nixkey.tailscale

import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.AndroidJUnit4
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Instrumented test for [TailscaleManager] lifecycle (start/stop)
 * using a [FakeTailscaleBackend] that simulates libtailscale behavior.
 */
@RunWith(AndroidJUnit4::class)
class TailscaleManagerTest {

    private lateinit var manager: TailscaleManager
    private lateinit var fakeBackend: FakeTailscaleBackend

    @Before
    fun setUp() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        fakeBackend = FakeTailscaleBackend()
        manager = TailscaleManager(fakeBackend, context)
        // Clear any persisted auth state from previous test runs
        manager.clearAuthKey()
    }

    @After
    fun tearDown() {
        manager.stop()
    }

    @Test
    fun startWithAuthKey_connectsAutomatically() {
        val oauthUrl = manager.start("tskey-auth-test123")

        assertNull("Should not need OAuth when auth key is provided", oauthUrl)
        assertTrue("Should be running after start", manager.isRunning())
        assertEquals("100.64.0.1", manager.getIp())
        assertTrue("Backend should have received start call", fakeBackend.started)
        assertEquals("tskey-auth-test123", fakeBackend.lastAuthKey)
    }

    @Test
    fun startWithoutAuthKey_returnsOAuthUrl() {
        fakeBackend.simulateOAuthRequired = true

        val oauthUrl = manager.start()

        assertNotNull("Should return OAuth URL when no auth key", oauthUrl)
        assertEquals("https://login.tailscale.com/authorize?nonce=test", oauthUrl)
        assertTrue("Should still be running during OAuth", manager.isRunning())
        assertNotNull("Should have pending OAuth URL", manager.getPendingOAuthUrl())
    }

    @Test
    fun startWithoutAuthKey_usesStoredKey() {
        // Store an auth key first
        manager.storeAuthKey("tskey-stored-key")

        val oauthUrl = manager.start()

        assertNull("Should use stored key, no OAuth needed", oauthUrl)
        assertTrue("Should be running", manager.isRunning())
        assertEquals("tskey-stored-key", fakeBackend.lastAuthKey)
    }

    @Test
    fun stopAfterStart_stopsBackend() {
        manager.start("tskey-auth-test")
        assertTrue("Should be running", manager.isRunning())

        manager.stop()

        assertFalse("Should not be running after stop", manager.isRunning())
        assertNull("IP should be null after stop", manager.getIp())
        assertTrue("Backend stop should have been called", fakeBackend.stopped)
    }

    @Test
    fun stopWhenNotRunning_isNoOp() {
        // Should not throw
        manager.stop()
        assertFalse("Should not be running", manager.isRunning())
        assertFalse("Backend stop should not have been called", fakeBackend.stopped)
    }

    @Test(expected = IllegalStateException::class)
    fun doubleStart_throws() {
        manager.start("tskey-auth-test")
        manager.start("tskey-auth-test2")
    }

    @Test
    fun onOAuthComplete_refreshesIp() {
        fakeBackend.simulateOAuthRequired = true
        manager.start()

        assertNull("IP should be null during OAuth", manager.getIp())

        // Simulate OAuth completing — backend now reports connected
        fakeBackend.simulateOAuthRequired = false
        fakeBackend.simulatedIp = "100.64.0.42"
        manager.onOAuthComplete()

        assertNull("Pending OAuth URL should be cleared", manager.getPendingOAuthUrl())
        assertEquals("100.64.0.42", manager.getIp())
    }

    @Test
    fun startPassesDataDir() {
        manager.start("tskey-auth-test")

        assertNotNull("Data dir should be set", fakeBackend.lastDataDir)
        assertTrue(
            "Data dir should be under app files",
            fakeBackend.lastDataDir!!.contains("tailscale")
        )
    }

    @Test
    fun clearAuthKey_removesStoredKey() {
        manager.storeAuthKey("tskey-to-remove")
        assertTrue("Should have stored key", manager.hasStoredAuthKey())

        manager.clearAuthKey()
        assertFalse("Should not have stored key after clear", manager.hasStoredAuthKey())
    }

    @Test
    fun startFailure_resetsRunningState() {
        fakeBackend.simulateStartFailure = true

        try {
            manager.start("tskey-auth-test")
            assertTrue("Should have thrown", false)
        } catch (e: RuntimeException) {
            assertEquals("Simulated start failure", e.message)
        }

        assertFalse("Should not be running after failed start", manager.isRunning())
    }

    @Test
    fun restartAfterStop_works() {
        manager.start("tskey-auth-test")
        assertTrue(manager.isRunning())

        manager.stop()
        assertFalse(manager.isRunning())

        // Reset fake backend state
        fakeBackend.started = false
        fakeBackend.stopped = false

        manager.start("tskey-auth-test2")
        assertTrue("Should be running after restart", manager.isRunning())
        assertEquals("tskey-auth-test2", fakeBackend.lastAuthKey)
    }
}

/**
 * Fake implementation of [TailscaleBackend] for testing.
 * Simulates libtailscale behavior without requiring the actual native library.
 */
class FakeTailscaleBackend : TailscaleBackend {

    var started = false
    var stopped = false
    var lastAuthKey: String? = null
    var lastDataDir: String? = null
    var simulateOAuthRequired = false
    var simulateStartFailure = false
    var simulatedIp: String? = "100.64.0.1"

    override fun start(authKey: String?, dataDir: String): String? {
        if (simulateStartFailure) {
            throw RuntimeException("Simulated start failure")
        }
        started = true
        stopped = false
        lastAuthKey = authKey
        lastDataDir = dataDir

        return if (authKey == null && simulateOAuthRequired) {
            "https://login.tailscale.com/authorize?nonce=test"
        } else {
            null
        }
    }

    override fun stop() {
        stopped = true
        started = false
    }

    override fun getIp(): String? {
        return if (started && !simulateOAuthRequired) simulatedIp else null
    }

    override fun isRunning(): Boolean = started && !stopped
}
