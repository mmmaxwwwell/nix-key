package com.nixkey.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.filters.LargeTest
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * E2E regression tests for the TailscaleAuth screen.
 * Uses UI Automator to interact with the full app on an emulator.
 */
@RunWith(AndroidJUnit4::class)
@LargeTest
class TailscaleAuthE2ETest {

    private lateinit var device: UiDevice
    private lateinit var helper: NixKeyE2EHelper

    @Before
    fun setup() {
        device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
        helper = NixKeyE2EHelper(device = device, maxRetries = 2)
        helper.waitForApp(timeout = 60_000L)
        navigateToTailscaleAuth()
    }

    /**
     * BUG-001: Auth key with whitespace must be rejected by client-side validation.
     *
     * Enter an auth key containing internal spaces, tap Connect, and assert
     * the "Invalid auth key format" error is displayed and the screen does
     * not navigate away.
     */
    @Test
    fun bug001_authKeyWithWhitespace_showsValidationError() {
        // Enter an auth key that contains internal whitespace
        val authKeyField = device.wait(
            Until.findObject(By.clazz("android.widget.EditText")),
            NixKeyE2EHelper.DEFAULT_TIMEOUT_MS
        )
        checkNotNull(authKeyField) { "Auth key EditText not found" }
        authKeyField.clear()
        authKeyField.text = "tskey-auth- invalid key"

        // Tap the Connect button
        val connectButton = device.wait(
            Until.findObject(By.text("Connect")),
            NixKeyE2EHelper.SHORT_TIMEOUT_MS
        )
        checkNotNull(connectButton) { "Connect button not found" }
        connectButton.click()

        // Assert: "Invalid auth key format" error text appears
        val errorFound = device.wait(
            Until.hasObject(By.textContains("Invalid auth key format")),
            NixKeyE2EHelper.DEFAULT_TIMEOUT_MS
        ) ?: false
        assertTrue("Expected 'Invalid auth key format' error to be displayed", errorFound)

        // Assert: still on TailscaleAuth screen (Connect button still visible)
        val stillOnAuthScreen = device.wait(
            Until.hasObject(By.text("Connect")),
            NixKeyE2EHelper.SHORT_TIMEOUT_MS
        ) ?: false
        assertTrue("Should remain on TailscaleAuth screen after validation error", stillOnAuthScreen)
    }

    /**
     * Navigate to the TailscaleAuth screen.
     * If the app starts on TailscaleAuth (not yet authenticated), we're already there.
     * If on ServerList (already authenticated), go via Settings > Re-authenticate.
     */
    private fun navigateToTailscaleAuth() {
        // Check if already on the TailscaleAuth screen
        val onAuthScreen = helper.waitForElement(
            By.text("Connect to Tailscale"),
            NixKeyE2EHelper.SHORT_TIMEOUT_MS
        )
        if (onAuthScreen) return

        // On ServerList — navigate via Settings gear icon
        val settingsButton = device.wait(
            Until.findObject(By.desc("Settings")),
            NixKeyE2EHelper.DEFAULT_TIMEOUT_MS
        )
        settingsButton?.click()

        // Tap "Re-authenticate" to get to TailscaleAuth screen
        val reAuthButton = device.wait(
            Until.findObject(By.text("Re-authenticate")),
            NixKeyE2EHelper.DEFAULT_TIMEOUT_MS
        )
        checkNotNull(reAuthButton) { "Re-authenticate button not found in Settings" }
        reAuthButton.click()

        // Wait for TailscaleAuth screen
        val arrived = helper.waitForElement(
            By.text("Connect to Tailscale"),
            NixKeyE2EHelper.DEFAULT_TIMEOUT_MS
        )
        check(arrived) { "Failed to navigate to TailscaleAuth screen" }
    }
}
