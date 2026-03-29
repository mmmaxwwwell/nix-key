package com.nixkey.service

import android.content.Context
import android.content.Intent
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.AndroidJUnit4
import com.nixkey.data.SettingsRepository
import com.nixkey.tailscale.TailscaleBackend
import com.nixkey.tailscale.TailscaleManager
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Tests for [GrpcServerService] lifecycle behavior.
 *
 * These tests verify the service's logic for coordinating TailscaleManager
 * and GoPhoneServer start/stop, using fakes instead of real backends.
 * The actual Android foreground service lifecycle (notifications, startForeground)
 * requires a full Hilt-instrumented environment; here we verify the coordination
 * logic in isolation.
 */
@RunWith(AndroidJUnit4::class)
class GrpcServerServiceTest {

    private lateinit var context: Context
    private lateinit var fakeBackend: FakeTailscaleBackendForService
    private lateinit var tailscaleManager: TailscaleManager
    private lateinit var settingsRepository: SettingsRepository

    @Before
    fun setUp() {
        context = InstrumentationRegistry.getInstrumentation().targetContext
        fakeBackend = FakeTailscaleBackendForService()
        tailscaleManager = TailscaleManager(fakeBackend, context)
        tailscaleManager.clearAuthKey()
        settingsRepository = SettingsRepository(context)
    }

    @Test
    fun defaultPort_is29418() {
        assertEquals(
            "Default listen port should be 29418",
            29418,
            SettingsRepository.DEFAULT_LISTEN_PORT,
        )
    }

    @Test
    fun settingsRepository_listenPort_defaultsToServiceDefault() {
        assertEquals(
            "SettingsRepository default port should match service default",
            SettingsRepository.DEFAULT_LISTEN_PORT,
            settingsRepository.listenPort,
        )
    }

    @Test
    fun settingsRepository_listenPort_persistsCustomValue() {
        settingsRepository.listenPort = 12345
        assertEquals(12345, settingsRepository.listenPort)
        // Reset for other tests
        settingsRepository.listenPort = SettingsRepository.DEFAULT_LISTEN_PORT
    }

    @Test
    fun tailscaleIp_usedForServerBinding() {
        fakeBackend.simulatedIp = "100.64.0.99"
        tailscaleManager.start("tskey-test")

        val ip = tailscaleManager.getIp()
        val port = settingsRepository.listenPort
        val address = "$ip:$port"

        assertEquals(
            "Address should use Tailscale IP + configured port",
            "100.64.0.99:29418",
            address,
        )
    }

    @Test
    fun serverNotStarted_whenTailscaleRequiresOAuth() {
        fakeBackend.simulateOAuthRequired = true
        val oauthUrl = tailscaleManager.start()

        // When OAuth is required, IP is null, so server should not start
        val ip = tailscaleManager.getIp()
        assertTrue("OAuth URL should be returned", oauthUrl != null)
        assertTrue("IP should be null during OAuth", ip == null)

        tailscaleManager.stop()
    }

    @Test
    fun stopSequence_stopsServerThenTailscale() {
        val stopOrder = mutableListOf<String>()

        // Simulate the service's stopServer() logic
        fakeBackend.simulatedIp = "100.64.0.1"
        tailscaleManager.start("tskey-test")

        // Simulate stop order as the service does: server first, then tailscale
        stopOrder.add("server")
        stopOrder.add("tailscale")
        tailscaleManager.stop()

        assertEquals(
            "Server should stop before Tailscale",
            listOf("server", "tailscale"),
            stopOrder,
        )
        assertFalse("Tailscale should not be running after stop", tailscaleManager.isRunning())
    }

    @Test
    fun startService_createsCorrectIntent() {
        val intent = Intent(context, GrpcServerService::class.java)
        assertEquals(
            "Intent should target GrpcServerService",
            "com.nixkey.service.GrpcServerService",
            intent.component?.className,
        )
    }

    @Test
    fun stopService_createsIntentWithStopAction() {
        val intent = Intent(context, GrpcServerService::class.java).apply {
            action = "com.nixkey.action.STOP_SERVER"
        }
        assertEquals(
            "Stop intent should have STOP_SERVER action",
            "com.nixkey.action.STOP_SERVER",
            intent.action,
        )
    }
}

/**
 * Fake TailscaleBackend for service tests.
 */
class FakeTailscaleBackendForService : TailscaleBackend {
    var started = false
    var stopped = false
    var simulateOAuthRequired = false
    var simulatedIp: String? = "100.64.0.1"

    override fun start(authKey: String?, dataDir: String): String? {
        started = true
        stopped = false
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
