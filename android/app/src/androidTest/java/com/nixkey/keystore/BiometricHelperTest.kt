package com.nixkey.keystore

import androidx.biometric.BiometricManager
import androidx.biometric.BiometricManager.Authenticators
import androidx.test.runner.AndroidJUnit4
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class BiometricHelperTest {

    private lateinit var mockBiometricManager: BiometricManager
    private lateinit var biometricHelper: BiometricHelper

    @Before
    fun setup() {
        mockBiometricManager = mockk()
        biometricHelper = BiometricHelper(mockBiometricManager)
    }

    // --- canAuthenticate tests ---

    @Test
    fun canAuthenticate_autoApprove_alwaysReturnsSuccess() {
        // AUTO_APPROVE never needs hardware, should always return success
        val result = biometricHelper.canAuthenticate(ConfirmationPolicy.AUTO_APPROVE)
        assertEquals(BiometricManager.BIOMETRIC_SUCCESS, result)
    }

    @Test
    fun canAuthenticate_biometric_delegatesToBiometricManager() {
        every {
            mockBiometricManager.canAuthenticate(Authenticators.BIOMETRIC_STRONG)
        } returns BiometricManager.BIOMETRIC_SUCCESS

        val result = biometricHelper.canAuthenticate(ConfirmationPolicy.BIOMETRIC)

        assertEquals(BiometricManager.BIOMETRIC_SUCCESS, result)
        verify { mockBiometricManager.canAuthenticate(Authenticators.BIOMETRIC_STRONG) }
    }

    @Test
    fun canAuthenticate_biometric_returnsErrorWhenNotEnrolled() {
        every {
            mockBiometricManager.canAuthenticate(Authenticators.BIOMETRIC_STRONG)
        } returns BiometricManager.BIOMETRIC_ERROR_NONE_ENROLLED

        val result = biometricHelper.canAuthenticate(ConfirmationPolicy.BIOMETRIC)

        assertEquals(BiometricManager.BIOMETRIC_ERROR_NONE_ENROLLED, result)
    }

    @Test
    fun canAuthenticate_biometric_returnsErrorWhenNoHardware() {
        every {
            mockBiometricManager.canAuthenticate(Authenticators.BIOMETRIC_STRONG)
        } returns BiometricManager.BIOMETRIC_ERROR_NO_HARDWARE

        val result = biometricHelper.canAuthenticate(ConfirmationPolicy.BIOMETRIC)

        assertEquals(BiometricManager.BIOMETRIC_ERROR_NO_HARDWARE, result)
    }

    @Test
    fun canAuthenticate_password_checksDeviceCredential() {
        every {
            mockBiometricManager.canAuthenticate(Authenticators.DEVICE_CREDENTIAL)
        } returns BiometricManager.BIOMETRIC_SUCCESS

        val result = biometricHelper.canAuthenticate(ConfirmationPolicy.PASSWORD)

        assertEquals(BiometricManager.BIOMETRIC_SUCCESS, result)
        verify { mockBiometricManager.canAuthenticate(Authenticators.DEVICE_CREDENTIAL) }
    }

    @Test
    fun canAuthenticate_alwaysAsk_checksDeviceCredential() {
        every {
            mockBiometricManager.canAuthenticate(Authenticators.DEVICE_CREDENTIAL)
        } returns BiometricManager.BIOMETRIC_SUCCESS

        val result = biometricHelper.canAuthenticate(ConfirmationPolicy.ALWAYS_ASK)

        assertEquals(BiometricManager.BIOMETRIC_SUCCESS, result)
        verify { mockBiometricManager.canAuthenticate(Authenticators.DEVICE_CREDENTIAL) }
    }

    @Test
    fun canAuthenticate_biometricPassword_checksBothAuthenticators() {
        val combined = Authenticators.BIOMETRIC_STRONG or Authenticators.DEVICE_CREDENTIAL
        every {
            mockBiometricManager.canAuthenticate(combined)
        } returns BiometricManager.BIOMETRIC_SUCCESS

        val result = biometricHelper.canAuthenticate(ConfirmationPolicy.BIOMETRIC_PASSWORD)

        assertEquals(BiometricManager.BIOMETRIC_SUCCESS, result)
        verify { mockBiometricManager.canAuthenticate(combined) }
    }

    @Test
    fun canAuthenticate_biometricPassword_returnsErrorWhenNeitherAvailable() {
        val combined = Authenticators.BIOMETRIC_STRONG or Authenticators.DEVICE_CREDENTIAL
        every {
            mockBiometricManager.canAuthenticate(combined)
        } returns BiometricManager.BIOMETRIC_ERROR_NO_HARDWARE

        val result = biometricHelper.canAuthenticate(ConfirmationPolicy.BIOMETRIC_PASSWORD)

        assertEquals(BiometricManager.BIOMETRIC_ERROR_NO_HARDWARE, result)
    }

    // --- authenticate tests ---

    @Test
    fun authenticate_autoApprove_immediatelySucceeds() {
        val latch = CountDownLatch(1)
        var capturedResult: AuthResult? = null

        // AUTO_APPROVE should succeed without needing an activity
        // We pass a mock activity since it won't actually be used
        val mockActivity = mockk<androidx.fragment.app.FragmentActivity>(relaxed = true)

        biometricHelper.authenticate(
            activity = mockActivity,
            policy = ConfirmationPolicy.AUTO_APPROVE,
            title = "Sign request"
        ) { result ->
            capturedResult = result
            latch.countDown()
        }

        assertTrue("Callback should fire immediately", latch.await(1, TimeUnit.SECONDS))
        assertTrue("Result should be Success", capturedResult is AuthResult.Success)
    }

    @Test
    fun authenticate_autoApprove_doesNotInteractWithBiometricManager() {
        val mockActivity = mockk<androidx.fragment.app.FragmentActivity>(relaxed = true)

        biometricHelper.authenticate(
            activity = mockActivity,
            policy = ConfirmationPolicy.AUTO_APPROVE,
            title = "Sign request"
        ) {}

        // BiometricManager should never be called for auto_approve
        verify(exactly = 0) { mockBiometricManager.canAuthenticate(any()) }
    }

    // --- requiresAutoApproveWarning tests ---

    @Test
    fun requiresAutoApproveWarning_alwaysReturnsTrue() {
        assertTrue(biometricHelper.requiresAutoApproveWarning())
    }

    // --- Policy mapping verification ---

    @Test
    fun allPoliciesHandled_canAuthenticate() {
        // Ensure every ConfirmationPolicy value is handled without exception
        every { mockBiometricManager.canAuthenticate(any()) } returns BiometricManager.BIOMETRIC_SUCCESS

        for (policy in ConfirmationPolicy.entries) {
            val result = biometricHelper.canAuthenticate(policy)
            assertEquals(
                "Policy $policy should return BIOMETRIC_SUCCESS when mocked",
                BiometricManager.BIOMETRIC_SUCCESS,
                result
            )
        }
    }
}
