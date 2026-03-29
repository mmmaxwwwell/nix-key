package com.nixkey.ui

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import com.nixkey.keystore.ConfirmationPolicy
import com.nixkey.keystore.KeyType
import com.nixkey.ui.screens.KeyDetailScreen
import com.nixkey.ui.theme.NixKeyTheme
import com.nixkey.ui.viewmodel.KeyDetailState
import com.nixkey.ui.viewmodel.KeyDetailViewModel
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Rule
import org.junit.Test

class KeyDetailScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun createMode_showsCreateButton() {
        val state = MutableStateFlow(KeyDetailState(isCreateMode = true))
        val viewModel = mockk<KeyDetailViewModel>(relaxed = true)
        every { viewModel.state } returns state
        every { viewModel.isCreateMode } returns true

        composeTestRule.setContent {
            NixKeyTheme {
                KeyDetailScreen(
                    onBack = {},
                    viewModel = viewModel,
                )
            }
        }

        composeTestRule.onNodeWithText("Create Key").assertIsDisplayed()
        composeTestRule.onNodeWithText("Create").assertIsDisplayed()
        composeTestRule.onNodeWithText("Key name").assertIsDisplayed()
    }

    @Test
    fun createMode_showsKeyTypeChips() {
        val state = MutableStateFlow(KeyDetailState(isCreateMode = true))
        val viewModel = mockk<KeyDetailViewModel>(relaxed = true)
        every { viewModel.state } returns state
        every { viewModel.isCreateMode } returns true

        composeTestRule.setContent {
            NixKeyTheme {
                KeyDetailScreen(
                    onBack = {},
                    viewModel = viewModel,
                )
            }
        }

        composeTestRule.onNodeWithText("ED25519").assertIsDisplayed()
        composeTestRule.onNodeWithText("ECDSA-P256").assertIsDisplayed()
    }

    @Test
    fun createMode_selectEcdsaKeyType() {
        val state = MutableStateFlow(KeyDetailState(isCreateMode = true))
        val viewModel = mockk<KeyDetailViewModel>(relaxed = true)
        every { viewModel.state } returns state
        every { viewModel.isCreateMode } returns true

        composeTestRule.setContent {
            NixKeyTheme {
                KeyDetailScreen(
                    onBack = {},
                    viewModel = viewModel,
                )
            }
        }

        composeTestRule.onNodeWithText("ECDSA-P256").performClick()
        verify { viewModel.setKeyType(KeyType.ECDSA_P256) }
    }

    @Test
    fun createMode_enterNameAndCreate() {
        val state = MutableStateFlow(KeyDetailState(isCreateMode = true))
        val viewModel = mockk<KeyDetailViewModel>(relaxed = true)
        every { viewModel.state } returns state
        every { viewModel.isCreateMode } returns true

        composeTestRule.setContent {
            NixKeyTheme {
                KeyDetailScreen(
                    onBack = {},
                    viewModel = viewModel,
                )
            }
        }

        composeTestRule.onNodeWithText("Key name").performTextInput("test-key")
        verify { viewModel.setDisplayName("test-key") }

        composeTestRule.onNodeWithText("Create").performClick()
        verify { viewModel.createKey() }
    }

    @Test
    fun viewMode_showsExportButtons() {
        val state = MutableStateFlow(
            KeyDetailState(
                isCreateMode = false,
                displayName = "my-key",
                keyType = KeyType.ED25519,
                publicKeyString = "ssh-ed25519 AAAA... my-key",
                keyInfo = com.nixkey.keystore.SshKeyInfo(
                    alias = "test_alias",
                    displayName = "my-key",
                    keyType = KeyType.ED25519,
                    fingerprint = "SHA256:abc123",
                    confirmationPolicy = ConfirmationPolicy.ALWAYS_ASK,
                    createdAt = java.time.Instant.now(),
                    wrappingKeyAlias = null,
                ),
            ),
        )
        val viewModel = mockk<KeyDetailViewModel>(relaxed = true)
        every { viewModel.state } returns state
        every { viewModel.isCreateMode } returns false

        composeTestRule.setContent {
            NixKeyTheme {
                KeyDetailScreen(
                    onBack = {},
                    viewModel = viewModel,
                )
            }
        }

        composeTestRule.onNodeWithText("Export Public Key").assertIsDisplayed()
        composeTestRule.onNodeWithText("Copy").assertIsDisplayed()
        composeTestRule.onNodeWithText("Share").assertIsDisplayed()
        composeTestRule.onNodeWithText("QR Code").assertIsDisplayed()
    }

    @Test
    fun viewMode_showsDeleteButton() {
        val state = MutableStateFlow(
            KeyDetailState(
                isCreateMode = false,
                displayName = "my-key",
                keyType = KeyType.ED25519,
                keyInfo = com.nixkey.keystore.SshKeyInfo(
                    alias = "test_alias",
                    displayName = "my-key",
                    keyType = KeyType.ED25519,
                    fingerprint = "SHA256:abc123",
                    confirmationPolicy = ConfirmationPolicy.ALWAYS_ASK,
                    createdAt = java.time.Instant.now(),
                    wrappingKeyAlias = null,
                ),
            ),
        )
        val viewModel = mockk<KeyDetailViewModel>(relaxed = true)
        every { viewModel.state } returns state
        every { viewModel.isCreateMode } returns false

        composeTestRule.setContent {
            NixKeyTheme {
                KeyDetailScreen(
                    onBack = {},
                    viewModel = viewModel,
                )
            }
        }

        composeTestRule.onNodeWithText("Delete Key").assertIsDisplayed()
    }

    @Test
    fun autoApproveWarning_isShown() {
        val state = MutableStateFlow(
            KeyDetailState(
                isCreateMode = true,
                showAutoApproveWarning = true,
            ),
        )
        val viewModel = mockk<KeyDetailViewModel>(relaxed = true)
        every { viewModel.state } returns state
        every { viewModel.isCreateMode } returns true

        composeTestRule.setContent {
            NixKeyTheme {
                KeyDetailScreen(
                    onBack = {},
                    viewModel = viewModel,
                )
            }
        }

        composeTestRule.onNodeWithText("Security Warning").assertIsDisplayed()
        composeTestRule.onNodeWithText("Enable Auto-Approve").assertIsDisplayed()
        composeTestRule.onNodeWithText("Cancel").assertIsDisplayed()
    }

    @Test
    fun autoApproveWarning_confirmEnables() {
        val state = MutableStateFlow(
            KeyDetailState(
                isCreateMode = true,
                showAutoApproveWarning = true,
            ),
        )
        val viewModel = mockk<KeyDetailViewModel>(relaxed = true)
        every { viewModel.state } returns state
        every { viewModel.isCreateMode } returns true

        composeTestRule.setContent {
            NixKeyTheme {
                KeyDetailScreen(
                    onBack = {},
                    viewModel = viewModel,
                )
            }
        }

        composeTestRule.onNodeWithText("Enable Auto-Approve").performClick()
        verify { viewModel.confirmAutoApprove() }
    }

    @Test
    fun viewMode_showsSaveWhenChanged() {
        val state = MutableStateFlow(
            KeyDetailState(
                isCreateMode = false,
                displayName = "edited-name",
                hasUnsavedChanges = true,
                keyType = KeyType.ED25519,
                keyInfo = com.nixkey.keystore.SshKeyInfo(
                    alias = "test_alias",
                    displayName = "my-key",
                    keyType = KeyType.ED25519,
                    fingerprint = "SHA256:abc123",
                    confirmationPolicy = ConfirmationPolicy.ALWAYS_ASK,
                    createdAt = java.time.Instant.now(),
                    wrappingKeyAlias = null,
                ),
            ),
        )
        val viewModel = mockk<KeyDetailViewModel>(relaxed = true)
        every { viewModel.state } returns state
        every { viewModel.isCreateMode } returns false

        composeTestRule.setContent {
            NixKeyTheme {
                KeyDetailScreen(
                    onBack = {},
                    viewModel = viewModel,
                )
            }
        }

        composeTestRule.onNodeWithText("Save").assertIsDisplayed()
        composeTestRule.onNodeWithText("Save").performClick()
        verify { viewModel.saveChanges() }
    }
}
