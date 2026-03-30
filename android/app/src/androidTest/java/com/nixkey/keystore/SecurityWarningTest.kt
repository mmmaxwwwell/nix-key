package com.nixkey.keystore

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.runner.AndroidJUnit4
import com.nixkey.ui.screens.KeyDetailScreen
import com.nixkey.ui.theme.NixKeyTheme
import com.nixkey.ui.viewmodel.KeyDetailState
import com.nixkey.ui.viewmodel.KeyDetailViewModel
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import kotlinx.coroutines.flow.MutableStateFlow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * T-AI-13: Security warnings for auto-approve signing and none-unlock policies (FR-046).
 *
 * Verifies that selecting AUTO_APPROVE or NONE triggers a security warning dialog
 * that the user must confirm before the policy is applied. Dismissing the dialog
 * leaves the policy unchanged.
 */
@RunWith(AndroidJUnit4::class)
class SecurityWarningTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    // --- ViewModel-level tests ---

    @Test
    fun setConfirmationPolicy_autoApprove_showsWarning() {
        val vm = createViewModel()
        vm.setConfirmationPolicy(ConfirmationPolicy.AUTO_APPROVE)

        val state = vm.state.value
        assertTrue("Auto-approve warning should be shown", state.showAutoApproveWarning)
        // Policy should NOT be applied yet
        assertEquals(ConfirmationPolicy.BIOMETRIC, state.confirmationPolicy)
    }

    @Test
    fun confirmAutoApprove_appliesPolicy() {
        val vm = createViewModel()
        vm.setConfirmationPolicy(ConfirmationPolicy.AUTO_APPROVE)
        vm.confirmAutoApprove()

        val state = vm.state.value
        assertFalse("Warning should be dismissed", state.showAutoApproveWarning)
        assertEquals(ConfirmationPolicy.AUTO_APPROVE, state.confirmationPolicy)
    }

    @Test
    fun dismissAutoApproveWarning_leavesOldPolicy() {
        val vm = createViewModel()
        vm.setConfirmationPolicy(ConfirmationPolicy.AUTO_APPROVE)
        vm.dismissAutoApproveWarning()

        val state = vm.state.value
        assertFalse("Warning should be dismissed", state.showAutoApproveWarning)
        assertEquals(ConfirmationPolicy.BIOMETRIC, state.confirmationPolicy)
    }

    @Test
    fun setUnlockPolicy_none_showsWarning() {
        val vm = createViewModel()
        vm.setUnlockPolicy(UnlockPolicy.NONE)

        val state = vm.state.value
        assertTrue("None-unlock warning should be shown", state.showNoneUnlockWarning)
        assertEquals(UnlockPolicy.PASSWORD, state.unlockPolicy)
    }

    @Test
    fun confirmNoneUnlock_appliesPolicy() {
        val vm = createViewModel()
        vm.setUnlockPolicy(UnlockPolicy.NONE)
        vm.confirmNoneUnlock()

        val state = vm.state.value
        assertFalse("Warning should be dismissed", state.showNoneUnlockWarning)
        assertEquals(UnlockPolicy.NONE, state.unlockPolicy)
    }

    @Test
    fun dismissNoneUnlockWarning_leavesOldPolicy() {
        val vm = createViewModel()
        vm.setUnlockPolicy(UnlockPolicy.NONE)
        vm.dismissNoneUnlockWarning()

        val state = vm.state.value
        assertFalse("Warning should be dismissed", state.showNoneUnlockWarning)
        assertEquals(UnlockPolicy.PASSWORD, state.unlockPolicy)
    }

    @Test
    fun otherPolicies_noWarning() {
        val vm = createViewModel()

        // Non-dangerous policies should apply immediately
        vm.setConfirmationPolicy(ConfirmationPolicy.PASSWORD)
        assertFalse(vm.state.value.showAutoApproveWarning)
        assertEquals(ConfirmationPolicy.PASSWORD, vm.state.value.confirmationPolicy)

        vm.setUnlockPolicy(UnlockPolicy.BIOMETRIC)
        assertFalse(vm.state.value.showNoneUnlockWarning)
        assertEquals(UnlockPolicy.BIOMETRIC, vm.state.value.unlockPolicy)
    }

    // --- Compose UI tests ---

    @Test
    fun autoApproveWarningDialog_rendersAndConfirms() {
        val state = MutableStateFlow(
            KeyDetailState(isCreateMode = true, showAutoApproveWarning = true),
        )
        val viewModel = mockk<KeyDetailViewModel>(relaxed = true)
        every { viewModel.state } returns state
        every { viewModel.isCreateMode } returns true

        composeTestRule.setContent {
            NixKeyTheme { KeyDetailScreen(onBack = {}, viewModel = viewModel) }
        }

        composeTestRule.onNodeWithText("Security Warning").assertIsDisplayed()
        composeTestRule.onNodeWithText("Enable Auto-Approve").performClick()
        verify { viewModel.confirmAutoApprove() }
    }

    @Test
    fun autoApproveWarningDialog_cancelDismisses() {
        val state = MutableStateFlow(
            KeyDetailState(isCreateMode = true, showAutoApproveWarning = true),
        )
        val viewModel = mockk<KeyDetailViewModel>(relaxed = true)
        every { viewModel.state } returns state
        every { viewModel.isCreateMode } returns true

        composeTestRule.setContent {
            NixKeyTheme { KeyDetailScreen(onBack = {}, viewModel = viewModel) }
        }

        composeTestRule.onNodeWithText("Cancel").performClick()
        verify { viewModel.dismissAutoApproveWarning() }
    }

    @Test
    fun noneUnlockWarningDialog_rendersAndConfirms() {
        val state = MutableStateFlow(
            KeyDetailState(isCreateMode = true, showNoneUnlockWarning = true),
        )
        val viewModel = mockk<KeyDetailViewModel>(relaxed = true)
        every { viewModel.state } returns state
        every { viewModel.isCreateMode } returns true

        composeTestRule.setContent {
            NixKeyTheme { KeyDetailScreen(onBack = {}, viewModel = viewModel) }
        }

        composeTestRule.onNodeWithText("Security Warning").assertIsDisplayed()
        composeTestRule.onNodeWithText("Disable Unlock").performClick()
        verify { viewModel.confirmNoneUnlock() }
    }

    @Test
    fun noneUnlockWarningDialog_cancelDismisses() {
        val state = MutableStateFlow(
            KeyDetailState(isCreateMode = true, showNoneUnlockWarning = true),
        )
        val viewModel = mockk<KeyDetailViewModel>(relaxed = true)
        every { viewModel.state } returns state
        every { viewModel.isCreateMode } returns true

        composeTestRule.setContent {
            NixKeyTheme { KeyDetailScreen(onBack = {}, viewModel = viewModel) }
        }

        composeTestRule.onNodeWithText("Cancel").performClick()
        verify { viewModel.dismissNoneUnlockWarning() }
    }

    @Test
    fun biometricHelper_warningFlags() {
        val helper = BiometricHelper()
        assertTrue("Auto-approve should always require warning", helper.requiresAutoApproveWarning())
        assertTrue("None-unlock should always require warning", helper.requiresNoneUnlockWarning())
    }

    private fun createViewModel(): KeyDetailViewModel {
        val keyManager = mockk<KeyManager>(relaxed = true)
        val keyUnlockManager = KeyUnlockManager()
        val savedState = androidx.lifecycle.SavedStateHandle(mapOf("keyId" to "new"))
        return KeyDetailViewModel(keyManager, keyUnlockManager, savedState)
    }
}
