package com.nixkey.ui

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.nixkey.data.PairedHost
import com.nixkey.keystore.SshKeyInfo
import com.nixkey.ui.screens.KeyListScreen
import com.nixkey.ui.screens.ServerListScreen
import com.nixkey.ui.screens.SettingsScreen
import com.nixkey.ui.theme.NixKeyTheme
import com.nixkey.ui.viewmodel.KeyListViewModel
import com.nixkey.ui.viewmodel.ServerListViewModel
import com.nixkey.ui.viewmodel.SettingsState
import com.nixkey.ui.viewmodel.SettingsViewModel
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class NavigationTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun serverListScreen_displaysCorrectly() {
        val viewModel = mockk<ServerListViewModel>(relaxed = true)
        every { viewModel.hosts } returns MutableStateFlow(emptyList())

        composeTestRule.setContent {
            NixKeyTheme {
                ServerListScreen(
                    onNavigateToSettings = {},
                    onNavigateToScanQr = {},
                    onNavigateToKeys = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithText("nix-key").assertIsDisplayed()
        composeTestRule.onNodeWithText("Scan QR Code").assertIsDisplayed()
        composeTestRule.onNodeWithText("No paired hosts yet").assertIsDisplayed()
    }

    @Test
    fun serverListScreen_settingsButtonCallsCallback() {
        val viewModel = mockk<ServerListViewModel>(relaxed = true)
        every { viewModel.hosts } returns MutableStateFlow(emptyList())
        var settingsTapped = false

        composeTestRule.setContent {
            NixKeyTheme {
                ServerListScreen(
                    onNavigateToSettings = { settingsTapped = true },
                    onNavigateToScanQr = {},
                    onNavigateToKeys = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Settings").performClick()
        assertTrue(settingsTapped)
    }

    @Test
    fun serverListScreen_scanQrCallsCallback() {
        val viewModel = mockk<ServerListViewModel>(relaxed = true)
        every { viewModel.hosts } returns MutableStateFlow(emptyList())
        var scanTapped = false

        composeTestRule.setContent {
            NixKeyTheme {
                ServerListScreen(
                    onNavigateToSettings = {},
                    onNavigateToScanQr = { scanTapped = true },
                    onNavigateToKeys = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithText("Scan QR Code").performClick()
        assertTrue(scanTapped)
    }

    @Test
    fun serverListScreen_hostRowCallsCallback() {
        val hosts = listOf(
            PairedHost(id = "host1", hostName = "workstation", tailscaleIp = "100.64.0.1")
        )
        val viewModel = mockk<ServerListViewModel>(relaxed = true)
        every { viewModel.hosts } returns MutableStateFlow(hosts)
        var navigatedHostId = ""

        composeTestRule.setContent {
            NixKeyTheme {
                ServerListScreen(
                    onNavigateToSettings = {},
                    onNavigateToScanQr = {},
                    onNavigateToKeys = { navigatedHostId = it },
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithText("workstation").performClick()
        assertEquals("host1", navigatedHostId)
    }

    @Test
    fun serverListScreen_showsHostDetails() {
        val hosts = listOf(
            PairedHost(id = "host1", hostName = "workstation", tailscaleIp = "100.64.0.1")
        )
        val viewModel = mockk<ServerListViewModel>(relaxed = true)
        every { viewModel.hosts } returns MutableStateFlow(hosts)

        composeTestRule.setContent {
            NixKeyTheme {
                ServerListScreen(
                    onNavigateToSettings = {},
                    onNavigateToScanQr = {},
                    onNavigateToKeys = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithText("workstation").assertIsDisplayed()
        composeTestRule.onNodeWithText("100.64.0.1").assertIsDisplayed()
    }

    @Test
    fun keyListScreen_showsEmptyState() {
        val viewModel = mockk<KeyListViewModel>(relaxed = true)
        every { viewModel.keys } returns MutableStateFlow(emptyList<SshKeyInfo>())

        composeTestRule.setContent {
            NixKeyTheme {
                KeyListScreen(
                    hostId = "host1",
                    onBack = {},
                    onNavigateToKeyDetail = {},
                    onNavigateToCreateKey = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithText("No keys yet").assertIsDisplayed()
        composeTestRule.onNodeWithText("Create one to get started").assertIsDisplayed()
    }

    @Test
    fun keyListScreen_fabCallsCreateCallback() {
        val viewModel = mockk<KeyListViewModel>(relaxed = true)
        every { viewModel.keys } returns MutableStateFlow(emptyList<SshKeyInfo>())
        var createCalled = false

        composeTestRule.setContent {
            NixKeyTheme {
                KeyListScreen(
                    hostId = "host1",
                    onBack = {},
                    onNavigateToKeyDetail = {},
                    onNavigateToCreateKey = { createCalled = true },
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Create Key").performClick()
        assertTrue(createCalled)
    }

    @Test
    fun keyListScreen_backCallsCallback() {
        val viewModel = mockk<KeyListViewModel>(relaxed = true)
        every { viewModel.keys } returns MutableStateFlow(emptyList<SshKeyInfo>())
        var backCalled = false

        composeTestRule.setContent {
            NixKeyTheme {
                KeyListScreen(
                    hostId = "host1",
                    onBack = { backCalled = true },
                    onNavigateToKeyDetail = {},
                    onNavigateToCreateKey = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Back").performClick()
        assertTrue(backCalled)
    }

    @Test
    fun settingsScreen_displaysCorrectly() {
        val viewModel = mockk<SettingsViewModel>(relaxed = true)
        every { viewModel.state } returns MutableStateFlow(SettingsState())

        composeTestRule.setContent {
            NixKeyTheme {
                SettingsScreen(
                    onBack = {},
                    viewModel = viewModel
                )
            }
        }

        composeTestRule.onNodeWithText("Settings").assertIsDisplayed()
        composeTestRule.onNodeWithText("Security").assertIsDisplayed()
        composeTestRule.onNodeWithText("Tracing").assertIsDisplayed()
    }
}
