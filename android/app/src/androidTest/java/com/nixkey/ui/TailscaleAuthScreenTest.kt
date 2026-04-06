package com.nixkey.ui

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import com.nixkey.ui.screens.TailscaleAuthContent
import com.nixkey.ui.theme.NixKeyTheme
import com.nixkey.ui.viewmodel.TailscaleAuthPhase
import com.nixkey.ui.viewmodel.TailscaleAuthState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class TailscaleAuthScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun inputPhase_displaysAllElements() {
        composeTestRule.setContent {
            NixKeyTheme {
                TailscaleAuthContent(
                    state = TailscaleAuthState(),
                    onAuthKeyChanged = {},
                    onConnectWithKey = {},
                    onConnectWithOAuth = {},
                    onOAuthComplete = {},
                    onRetry = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Connect to Tailscale").assertIsDisplayed()
        composeTestRule.onNodeWithText("Connect").assertIsDisplayed()
        composeTestRule.onNodeWithText("Sign in with Tailscale").assertIsDisplayed()
        composeTestRule.onNodeWithText("or").assertIsDisplayed()
    }

    @Test
    fun inputPhase_connectButtonCallsCallback() {
        var connectCalled = false

        composeTestRule.setContent {
            NixKeyTheme {
                TailscaleAuthContent(
                    state = TailscaleAuthState(authKey = "tskey-auth-test"),
                    onAuthKeyChanged = {},
                    onConnectWithKey = { connectCalled = true },
                    onConnectWithOAuth = {},
                    onOAuthComplete = {},
                    onRetry = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Connect").performClick()
        assertTrue(connectCalled)
    }

    @Test
    fun inputPhase_oauthButtonCallsCallback() {
        var oauthCalled = false

        composeTestRule.setContent {
            NixKeyTheme {
                TailscaleAuthContent(
                    state = TailscaleAuthState(),
                    onAuthKeyChanged = {},
                    onConnectWithKey = {},
                    onConnectWithOAuth = { oauthCalled = true },
                    onOAuthComplete = {},
                    onRetry = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Sign in with Tailscale").performClick()
        assertTrue(oauthCalled)
    }

    @Test
    fun inputPhase_authKeyChangedCallback() {
        var capturedKey = ""

        composeTestRule.setContent {
            NixKeyTheme {
                TailscaleAuthContent(
                    state = TailscaleAuthState(),
                    onAuthKeyChanged = { capturedKey = it },
                    onConnectWithKey = {},
                    onConnectWithOAuth = {},
                    onOAuthComplete = {},
                    onRetry = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Auth Key").performTextInput("tskey-auth-abc")
        assertEquals("tskey-auth-abc", capturedKey)
    }

    @Test
    fun inputPhase_showsErrorWhenPresent() {
        composeTestRule.setContent {
            NixKeyTheme {
                TailscaleAuthContent(
                    state = TailscaleAuthState(error = "Auth key cannot be empty"),
                    onAuthKeyChanged = {},
                    onConnectWithKey = {},
                    onConnectWithOAuth = {},
                    onOAuthComplete = {},
                    onRetry = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Auth key cannot be empty").assertIsDisplayed()
    }

    @Test
    fun connectingPhase_showsProgressIndicator() {
        composeTestRule.setContent {
            NixKeyTheme {
                TailscaleAuthContent(
                    state = TailscaleAuthState(phase = TailscaleAuthPhase.CONNECTING),
                    onAuthKeyChanged = {},
                    onConnectWithKey = {},
                    onConnectWithOAuth = {},
                    onOAuthComplete = {},
                    onRetry = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Joining Tailnet...").assertIsDisplayed()
        // Connect button should not be visible in connecting phase
        composeTestRule.onNodeWithText("Connect").assertDoesNotExist()
    }

    @Test
    fun oauthPhase_showsSignedInButton() {
        composeTestRule.setContent {
            NixKeyTheme {
                TailscaleAuthContent(
                    state = TailscaleAuthState(
                        phase = TailscaleAuthPhase.OAUTH_REQUIRED,
                        oauthUrl = "https://login.tailscale.com/test"
                    ),
                    onAuthKeyChanged = {},
                    onConnectWithKey = {},
                    onConnectWithOAuth = {},
                    onOAuthComplete = {},
                    onRetry = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Complete sign-in in your browser, then tap below.")
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("I've signed in").assertIsDisplayed()
    }

    @Test
    fun oauthPhase_signedInCallsCallback() {
        var oauthCompleteCalled = false

        composeTestRule.setContent {
            NixKeyTheme {
                TailscaleAuthContent(
                    state = TailscaleAuthState(
                        phase = TailscaleAuthPhase.OAUTH_REQUIRED,
                        oauthUrl = "https://login.tailscale.com/test"
                    ),
                    onAuthKeyChanged = {},
                    onConnectWithKey = {},
                    onConnectWithOAuth = {},
                    onOAuthComplete = { oauthCompleteCalled = true },
                    onRetry = {}
                )
            }
        }

        composeTestRule.onNodeWithText("I've signed in").performClick()
        assertTrue(oauthCompleteCalled)
    }

    @Test
    fun errorPhase_showsErrorAndRetryButton() {
        composeTestRule.setContent {
            NixKeyTheme {
                TailscaleAuthContent(
                    state = TailscaleAuthState(
                        phase = TailscaleAuthPhase.ERROR,
                        error = "Connection failed: timeout"
                    ),
                    onAuthKeyChanged = {},
                    onConnectWithKey = {},
                    onConnectWithOAuth = {},
                    onOAuthComplete = {},
                    onRetry = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Connection failed: timeout").assertIsDisplayed()
        composeTestRule.onNodeWithText("Retry").assertIsDisplayed()
    }

    @Test
    fun errorPhase_retryCallsCallback() {
        var retryCalled = false

        composeTestRule.setContent {
            NixKeyTheme {
                TailscaleAuthContent(
                    state = TailscaleAuthState(
                        phase = TailscaleAuthPhase.ERROR,
                        error = "Connection failed"
                    ),
                    onAuthKeyChanged = {},
                    onConnectWithKey = {},
                    onConnectWithOAuth = {},
                    onOAuthComplete = {},
                    onRetry = { retryCalled = true }
                )
            }
        }

        composeTestRule.onNodeWithText("Retry").performClick()
        assertTrue(retryCalled)
    }
}
