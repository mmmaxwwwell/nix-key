package com.nixkey.pairing

import android.content.Intent
import android.net.Uri
import android.util.Base64
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.test.runner.AndroidJUnit4
import com.nixkey.MainActivity
import com.nixkey.data.HostRepository
import com.nixkey.data.SettingsRepository
import com.nixkey.ui.screens.PairingScreen
import com.nixkey.ui.theme.NixKeyTheme
import com.nixkey.ui.viewmodel.PairingViewModel
import io.mockk.every
import io.mockk.mockk
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Instrumented test for the test-mode deep link pairing (T064, FR-022).
 *
 * Verifies that:
 * 1. `nix-key://pair?payload=<base64>` intents are correctly parsed
 * 2. PairingScreen with an initial payload skips camera scanning
 *    and shows the host confirmation dialog with correct host info
 * 3. Non-matching intents are ignored
 */
@RunWith(AndroidJUnit4::class)
class DeepLinkPairingTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun extractPairPayload_validDeepLink_returnsPayload() {
        val payload = createBase64Payload("test-host.local", 8443)
        val intent = Intent(Intent.ACTION_VIEW).apply {
            data = Uri.parse("nix-key://pair?payload=$payload")
        }

        val result = MainActivity.extractPairPayload(intent)

        assertNotNull("Payload should be extracted from valid deep link", result)
        assertEquals(payload, result)
    }

    @Test
    fun extractPairPayload_wrongScheme_returnsNull() {
        val intent = Intent(Intent.ACTION_VIEW).apply {
            data = Uri.parse("https://pair?payload=abc")
        }

        val result = MainActivity.extractPairPayload(intent)
        assertNull("Wrong scheme should return null", result)
    }

    @Test
    fun extractPairPayload_wrongHost_returnsNull() {
        val intent = Intent(Intent.ACTION_VIEW).apply {
            data = Uri.parse("nix-key://other?payload=abc")
        }

        val result = MainActivity.extractPairPayload(intent)
        assertNull("Wrong host should return null", result)
    }

    @Test
    fun extractPairPayload_noData_returnsNull() {
        val intent = Intent(Intent.ACTION_MAIN)

        val result = MainActivity.extractPairPayload(intent)
        assertNull("Intent without data should return null", result)
    }

    @Test
    fun extractPairPayload_nullIntent_returnsNull() {
        val result = MainActivity.extractPairPayload(null)
        assertNull("Null intent should return null", result)
    }

    @Test
    fun pairingScreen_withInitialPayload_showsHostConfirmation() {
        val hostName = "nixos-workstation"
        val payload = createBase64Payload(hostName, 12345)

        val hostRepository = mockk<HostRepository>(relaxed = true)
        every { hostRepository.listHosts() } returns emptyList()
        val settingsRepository = mockk<SettingsRepository>(relaxed = true)
        every { settingsRepository.listenPort } returns 29418
        val tailscaleManager = mockk<com.nixkey.tailscale.TailscaleManager>(relaxed = true)
        val pairingClient = mockk<PairingClient>(relaxed = true)

        val viewModel = PairingViewModel(
            hostRepository = hostRepository,
            settingsRepository = settingsRepository,
            tailscaleManager = tailscaleManager,
            pairingClient = pairingClient
        )

        composeTestRule.setContent {
            NixKeyTheme {
                PairingScreen(
                    onBack = {},
                    onPairingComplete = {},
                    initialPayload = payload,
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.waitForIdle()

        // Verify the host confirmation dialog is shown with correct host name
        composeTestRule.onNodeWithText("Pair with host?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Connect to $hostName?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Accept").assertIsDisplayed()
        composeTestRule.onNodeWithText("Deny").assertIsDisplayed()
    }

    @Test
    fun pairingScreen_withInitialPayload_withOtel_showsHostFirst() {
        val hostName = "nixos-otel-host"
        val payload = createBase64Payload(hostName, 8080, otel = "100.64.0.1:4317")

        val hostRepository = mockk<HostRepository>(relaxed = true)
        every { hostRepository.listHosts() } returns emptyList()
        val settingsRepository = mockk<SettingsRepository>(relaxed = true)
        every { settingsRepository.listenPort } returns 29418
        val tailscaleManager = mockk<com.nixkey.tailscale.TailscaleManager>(relaxed = true)
        val pairingClient = mockk<PairingClient>(relaxed = true)

        val viewModel = PairingViewModel(
            hostRepository = hostRepository,
            settingsRepository = settingsRepository,
            tailscaleManager = tailscaleManager,
            pairingClient = pairingClient
        )

        composeTestRule.setContent {
            NixKeyTheme {
                PairingScreen(
                    onBack = {},
                    onPairingComplete = {},
                    initialPayload = payload,
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.waitForIdle()

        // Should show host confirmation first (even with otel in payload)
        composeTestRule.onNodeWithText("Pair with host?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Connect to $hostName?").assertIsDisplayed()
    }

    @Test
    fun pairingScreen_withoutInitialPayload_showsScanner() {
        val hostRepository = mockk<HostRepository>(relaxed = true)
        every { hostRepository.listHosts() } returns emptyList()
        val settingsRepository = mockk<SettingsRepository>(relaxed = true)
        every { settingsRepository.listenPort } returns 29418
        val tailscaleManager = mockk<com.nixkey.tailscale.TailscaleManager>(relaxed = true)
        val pairingClient = mockk<PairingClient>(relaxed = true)

        val viewModel = PairingViewModel(
            hostRepository = hostRepository,
            settingsRepository = settingsRepository,
            tailscaleManager = tailscaleManager,
            pairingClient = pairingClient
        )

        composeTestRule.setContent {
            NixKeyTheme {
                PairingScreen(
                    onBack = {},
                    onPairingComplete = {},
                    initialPayload = null,
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.waitForIdle()

        // Without payload, should show pairing screen title (scanner mode)
        composeTestRule.onNodeWithText("Pair with Host").assertIsDisplayed()
    }

    private fun createBase64Payload(
        host: String,
        port: Int,
        cert: String = "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----",
        token: String = "test-token-abc123",
        otel: String? = null
    ): String {
        val json = JSONObject().apply {
            put("v", 1)
            put("host", host)
            put("port", port)
            put("cert", cert)
            put("token", token)
            if (otel != null) put("otel", otel)
        }
        return Base64.encodeToString(json.toString().toByteArray(), Base64.NO_WRAP)
    }
}
