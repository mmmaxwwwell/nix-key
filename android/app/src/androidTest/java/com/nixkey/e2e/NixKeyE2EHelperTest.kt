package com.nixkey.e2e

import android.util.Base64
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.AndroidJUnit4
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import org.json.JSONObject
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Self-test for [NixKeyE2EHelper] that exercises each helper method against the app
 * running on a local emulator.
 *
 * Prerequisites:
 * - App must be installed on the emulator (debug build)
 * - Emulator must be booted and accessible via adb
 *
 * These tests validate the helper methods work correctly with the actual app UI.
 * They are designed to be run as part of the E2E test suite (T066).
 */
@RunWith(AndroidJUnit4::class)
class NixKeyE2EHelperTest {

    private lateinit var helper: NixKeyE2EHelper
    private lateinit var device: UiDevice

    @Before
    fun setUp() {
        device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
        helper = NixKeyE2EHelper(device = device, maxRetries = 1)
    }

    @Test
    fun helperInstantiates() {
        assertNotNull("Helper should be instantiated", helper)
    }

    @Test
    fun waitForApp_launchesAndFindsApp() {
        val result = helper.waitForApp(timeout = 60_000L)
        assertTrue("App should be visible after launch", result)
    }

    @Test
    fun waitForElement_findsExistingElement() {
        helper.waitForApp(timeout = 60_000L)
        // After launching, the app shows either TailscaleAuthScreen or ServerListScreen.
        // Both have recognizable text elements.
        val foundTailscale = helper.waitForElement(
            By.text("Connect to Tailscale"),
            NixKeyE2EHelper.SHORT_TIMEOUT_MS
        )
        val foundServerList = helper.waitForElement(
            By.text("nix-key"),
            NixKeyE2EHelper.SHORT_TIMEOUT_MS
        )
        assertTrue(
            "Should find either Tailscale auth screen or server list",
            foundTailscale || foundServerList
        )
    }

    @Test
    fun waitForElement_returnsFalseForMissingElement() {
        helper.waitForApp(timeout = 60_000L)
        val result = helper.waitForElement(
            By.text("THIS_ELEMENT_DOES_NOT_EXIST_ANYWHERE_12345"),
            1_000L
        )
        assertFalse("Should return false for non-existent element", result)
    }

    @Test
    fun pairWithHost_showsConfirmationDialog() {
        helper.waitForApp(timeout = 60_000L)
        // Send a deep link intent — the confirmation dialog should appear
        val payload = createBase64Payload("e2e-test-host", 8443)

        // We use a helper with retries=1 to keep the test fast.
        // The deep link should open the pairing confirmation dialog.
        // We don't tap Accept here (the pairing server isn't running),
        // so we just verify the dialog appears.
        val found = helper.waitForElement(By.text("Pair with host?"), 1_000L) ||
            run {
                // Send the intent manually to verify dialog appears
                val intent = android.content.Intent(
                    android.content.Intent.ACTION_VIEW,
                    android.net.Uri.parse("nix-key://pair?payload=$payload")
                ).apply {
                    setPackage("com.nixkey")
                    addFlags(android.content.Intent.FLAG_ACTIVITY_NEW_TASK)
                }
                InstrumentationRegistry.getInstrumentation().targetContext.startActivity(intent)
                helper.waitForElement(
                    By.text("Pair with host?"),
                    NixKeyE2EHelper.DEFAULT_TIMEOUT_MS
                )
            }
        assertTrue("Pairing confirmation dialog should appear after deep link", found)
    }

    @Test
    fun enterTailscaleAuthKey_findsInputField() {
        helper.waitForApp(timeout = 60_000L)
        // This test is only meaningful when the app starts on TailscaleAuthScreen
        val onAuthScreen = helper.waitForElement(
            By.text("Connect to Tailscale"),
            NixKeyE2EHelper.SHORT_TIMEOUT_MS
        )
        if (!onAuthScreen) {
            // App already has a stored auth key — skip this test
            return
        }
        // Verify the EditText (auth key field) is present
        val hasInput = helper.waitForElement(
            By.clazz("android.widget.EditText"),
            NixKeyE2EHelper.SHORT_TIMEOUT_MS
        )
        assertTrue("Auth key input field should be present", hasInput)
    }

    @Test
    fun approveSignRequest_returnsFalseWhenNoDialog() {
        helper.waitForApp(timeout = 60_000L)
        // No sign request is pending, so approveSignRequest should return false
        val shortHelper = NixKeyE2EHelper(device = device, maxRetries = 1)
        val result = shortHelper.approveSignRequest(timeout = 2_000L)
        assertFalse("Should return false when no sign request dialog is shown", result)
    }

    @Test
    fun denySignRequest_returnsFalseWhenNoDialog() {
        helper.waitForApp(timeout = 60_000L)
        val shortHelper = NixKeyE2EHelper(device = device, maxRetries = 1)
        val result = shortHelper.denySignRequest()
        assertFalse("Should return false when no sign request dialog is shown", result)
    }

    private fun createBase64Payload(
        host: String,
        port: Int,
        cert: String = "-----BEGIN CERTIFICATE-----\nTEST\n-----END CERTIFICATE-----",
        token: String = "test-token-e2e"
    ): String {
        val json = JSONObject().apply {
            put("v", 1)
            put("host", host)
            put("port", port)
            put("cert", cert)
            put("token", token)
        }
        return Base64.encodeToString(json.toString().toByteArray(), Base64.NO_WRAP)
    }
}
