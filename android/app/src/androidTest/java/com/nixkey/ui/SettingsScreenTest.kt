package com.nixkey.ui

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.nixkey.ui.screens.SettingsScreen
import com.nixkey.ui.theme.NixKeyTheme
import com.nixkey.ui.viewmodel.SettingsState
import com.nixkey.ui.viewmodel.SettingsViewModel
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Rule
import org.junit.Test

class SettingsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun settingsScreen_showsAllSections() {
        val state = MutableStateFlow(SettingsState())
        val viewModel = mockk<SettingsViewModel>(relaxed = true)
        every { viewModel.state } returns state

        composeTestRule.setContent {
            NixKeyTheme {
                SettingsScreen(
                    onBack = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithText("Security").assertIsDisplayed()
        composeTestRule.onNodeWithText("Allow key listing").assertIsDisplayed()
        composeTestRule.onNodeWithText("Default confirmation policy").assertIsDisplayed()
        composeTestRule.onNodeWithText("Tracing").assertIsDisplayed()
        composeTestRule.onNodeWithText("Enable tracing").assertIsDisplayed()
    }

    @Test
    fun settingsScreen_otelEndpointHiddenWhenDisabled() {
        val state = MutableStateFlow(SettingsState(otelEnabled = false))
        val viewModel = mockk<SettingsViewModel>(relaxed = true)
        every { viewModel.state } returns state

        composeTestRule.setContent {
            NixKeyTheme {
                SettingsScreen(
                    onBack = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithText("OTEL endpoint").assertDoesNotExist()
    }

    @Test
    fun settingsScreen_otelEndpointShownWhenEnabled() {
        val state = MutableStateFlow(SettingsState(otelEnabled = true, otelEndpoint = "localhost:4317"))
        val viewModel = mockk<SettingsViewModel>(relaxed = true)
        every { viewModel.state } returns state

        composeTestRule.setContent {
            NixKeyTheme {
                SettingsScreen(
                    onBack = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithText("OTEL endpoint").assertIsDisplayed()
    }

    @Test
    fun settingsScreen_backNavigates() {
        val state = MutableStateFlow(SettingsState())
        val viewModel = mockk<SettingsViewModel>(relaxed = true)
        every { viewModel.state } returns state
        var backCalled = false

        composeTestRule.setContent {
            NixKeyTheme {
                SettingsScreen(
                    onBack = { backCalled = true },
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Back").performClick()
        assert(backCalled)
    }
}
