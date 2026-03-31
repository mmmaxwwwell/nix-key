package com.nixkey.e2e

import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.BySelector
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until

/**
 * Reusable UI Automator helper for nix-key Android E2E tests.
 *
 * Each method includes retry logic (3 attempts) to handle UI flakiness
 * common in emulator-based testing.
 */
class NixKeyE2EHelper(
    private val device: UiDevice = UiDevice.getInstance(
        InstrumentationRegistry.getInstrumentation()
    ),
    private val maxRetries: Int = 3
) {
    private val context: Context
        get() = InstrumentationRegistry.getInstrumentation().targetContext

    private val packageName: String = "com.nixkey"

    companion object {
        const val DEFAULT_TIMEOUT_MS = 10_000L
        const val SHORT_TIMEOUT_MS = 5_000L
        const val RETRY_DELAY_MS = 1_000L
    }

    /**
     * Wait for MainActivity to be visible.
     */
    fun waitForApp(timeout: Long = DEFAULT_TIMEOUT_MS): Boolean = retry("waitForApp") {
        val launchIntent = context.packageManager.getLaunchIntentForPackage(packageName)
            ?: throw IllegalStateException("Launch intent not found for $packageName")
        launchIntent.addFlags(Intent.FLAG_ACTIVITY_CLEAR_TASK)
        context.startActivity(launchIntent)
        device.wait(Until.hasObject(By.pkg(packageName).depth(0)), timeout)
            ?: false
    }

    /**
     * Navigate to key management screen by tapping the first host card in the server list.
     * Assumes the app is on the ServerListScreen with at least one paired host.
     */
    fun navigateToKeys(): Boolean = retry("navigateToKeys") {
        // ServerListScreen shows host cards; tapping one navigates to KeyListScreen
        // Wait for the server list to be visible (title "nix-key")
        waitForElement(By.text("nix-key"), DEFAULT_TIMEOUT_MS)

        // Find and tap the first host card - host cards contain the hostname text
        val hostCard = device.wait(
            Until.findObject(By.res(packageName, "host_card").clickable(true)),
            DEFAULT_TIMEOUT_MS
        )
        if (hostCard != null) {
            hostCard.click()
        } else {
            // Fallback: tap any clickable card-like element below the top bar
            val anyCard = device.findObject(By.clazz("android.view.View").clickable(true))
            anyCard?.click() ?: return@retry false
        }

        // Wait for KeyListScreen to appear (title "Keys")
        waitForElement(By.text("Keys"), DEFAULT_TIMEOUT_MS)
    }

    /**
     * Create a new key: tap FAB, fill the form, submit, wait for the key to appear in the list.
     *
     * @param name Display name for the new key
     * @param type Key type: "ed25519" or "ecdsa_p256"
     */
    fun createKey(name: String, type: String): Boolean = retry("createKey($name, $type)") {
        // Wait for KeyListScreen
        waitForElement(By.text("Keys"), DEFAULT_TIMEOUT_MS)

        // Tap the FAB (Create Key content description)
        val fab = device.wait(Until.findObject(By.desc("Create Key")), DEFAULT_TIMEOUT_MS)
            ?: return@retry false
        fab.click()

        // Wait for the KeyDetailScreen in create mode (title "Create Key")
        waitForElement(By.text("Create Key"), DEFAULT_TIMEOUT_MS)

        // Fill in the key name field
        val nameField = device.wait(Until.findObject(By.text("Key name")), SHORT_TIMEOUT_MS)
            ?: return@retry false
        // The OutlinedTextField label is "Key name"; find the actual editable field
        val editField = device.findObject(By.clazz("android.widget.EditText"))
            ?: return@retry false
        editField.clear()
        editField.text = name

        // Select key type chip
        val chipLabel = when (type.lowercase()) {
            "ed25519" -> "ED25519"
            "ecdsa_p256", "ecdsa-p256", "ecdsa" -> "ECDSA-P256"
            else -> type.uppercase().replace("_", "-")
        }
        val typeChip = device.findObject(By.text(chipLabel))
        typeChip?.click()

        // Tap "Create" button
        val createButton = device.wait(Until.findObject(By.text("Create")), SHORT_TIMEOUT_MS)
            ?: return@retry false
        createButton.click()

        // Wait for navigation back to KeyListScreen, then wait for the key name to appear
        device.wait(Until.hasObject(By.text(name)), DEFAULT_TIMEOUT_MS) ?: false
    }

    /**
     * Pair with a host using the test-mode deep link (T064).
     *
     * Sends the `nix-key://pair?payload=<base64>` intent, waits for the confirmation dialog,
     * and taps "Accept".
     *
     * @param qrPayload Base64-encoded QR payload JSON
     */
    fun pairWithHost(qrPayload: String): Boolean = retry("pairWithHost") {
        // Send deep link intent
        val deepLinkUri = Uri.parse("nix-key://pair?payload=$qrPayload")
        val intent = Intent(Intent.ACTION_VIEW, deepLinkUri).apply {
            setPackage(packageName)
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        context.startActivity(intent)

        // Wait for the confirmation dialog ("Pair with host?")
        waitForElement(By.text("Pair with host?"), DEFAULT_TIMEOUT_MS)

        // Tap "Accept"
        val acceptButton = device.wait(Until.findObject(By.text("Accept")), SHORT_TIMEOUT_MS)
            ?: return@retry false
        acceptButton.click()

        // Wait for pairing progress or success (dialog should disappear)
        device.wait(Until.gone(By.text("Pair with host?")), DEFAULT_TIMEOUT_MS) ?: false
    }

    /**
     * Wait for the sign request dialog to appear, then tap "Approve".
     *
     * @param timeout How long to wait for the sign request dialog to appear
     */
    fun approveSignRequest(timeout: Long = DEFAULT_TIMEOUT_MS): Boolean = retry("approveSignRequest") {
        // Wait for the sign request dialog (title "Sign Request")
        waitForElement(By.text("Sign Request"), timeout)

        // Tap "Approve"
        val approveButton = device.wait(
            Until.findObject(By.text("Approve")),
            SHORT_TIMEOUT_MS
        ) ?: return@retry false
        approveButton.click()

        // Wait for dialog to disappear
        device.wait(Until.gone(By.text("Sign Request")), SHORT_TIMEOUT_MS) ?: false
    }

    /**
     * Wait for the sign request dialog to appear, then tap "Deny".
     */
    fun denySignRequest(): Boolean = retry("denySignRequest") {
        // Wait for the sign request dialog
        waitForElement(By.text("Sign Request"), DEFAULT_TIMEOUT_MS)

        // Tap "Deny"
        val denyButton = device.wait(
            Until.findObject(By.text("Deny")),
            SHORT_TIMEOUT_MS
        ) ?: return@retry false
        denyButton.click()

        // Wait for dialog to disappear
        device.wait(Until.gone(By.text("Sign Request")), SHORT_TIMEOUT_MS) ?: false
    }

    /**
     * On the Tailscale auth screen, enter the auth key, tap Connect, and wait for success.
     *
     * @param key Tailscale auth key (e.g., "tskey-auth-...")
     */
    fun enterTailscaleAuthKey(key: String): Boolean = retry("enterTailscaleAuthKey") {
        // Wait for the Tailscale auth screen
        waitForElement(By.text("Connect to Tailscale"), DEFAULT_TIMEOUT_MS)

        // Find the auth key text field and enter the key
        val editField = device.wait(
            Until.findObject(By.clazz("android.widget.EditText")),
            SHORT_TIMEOUT_MS
        ) ?: return@retry false
        editField.clear()
        editField.text = key

        // Tap "Connect" button
        val connectButton = device.wait(
            Until.findObject(By.text("Connect")),
            SHORT_TIMEOUT_MS
        ) ?: return@retry false
        connectButton.click()

        // Wait for the auth screen to transition away (either success -> ServerListScreen,
        // or the "Joining Tailnet..." progress indicator)
        val joinedOrList = device.wait(
            Until.hasObject(By.textStartsWith("nix-key")),
            DEFAULT_TIMEOUT_MS
        ) ?: device.wait(
            Until.hasObject(By.text("Joining Tailnet...")),
            DEFAULT_TIMEOUT_MS / 2
        ) ?: false
        joinedOrList
    }

    /**
     * Generic wait for an element matching the given selector.
     *
     * @param selector UI Automator selector
     * @param timeout Timeout in milliseconds
     * @return true if the element was found within the timeout
     */
    fun waitForElement(selector: BySelector, timeout: Long = DEFAULT_TIMEOUT_MS): Boolean =
        device.wait(Until.hasObject(selector), timeout) ?: false

    /**
     * Retry an action up to [maxRetries] times, with a delay between attempts.
     * Returns the result of the first successful attempt, or the result of the last attempt.
     */
    private fun <T> retry(actionName: String, action: () -> T): T {
        var lastResult: T? = null
        var lastException: Exception? = null
        for (attempt in 1..maxRetries) {
            try {
                lastResult = action()
                if (lastResult is Boolean && lastResult == true) {
                    return lastResult
                }
                if (lastResult !is Boolean) {
                    return lastResult
                }
            } catch (e: Exception) {
                lastException = e
            }
            if (attempt < maxRetries) {
                Thread.sleep(RETRY_DELAY_MS)
            }
        }
        if (lastException != null && lastResult == null) {
            throw lastException
        }
        @Suppress("UNCHECKED_CAST")
        return lastResult as T
    }
}
