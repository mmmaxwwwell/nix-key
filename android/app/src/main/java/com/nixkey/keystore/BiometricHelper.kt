package com.nixkey.keystore

import androidx.biometric.BiometricManager
import androidx.biometric.BiometricManager.Authenticators
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import timber.log.Timber
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Result of a biometric/confirmation authentication attempt.
 */
sealed class AuthResult {
    data object Success : AuthResult()
    data class Failure(val errorCode: Int, val message: String) : AuthResult()
    data object Cancelled : AuthResult()
}

/**
 * Wraps the BiometricPrompt API to support per-key confirmation policies.
 *
 * Policy mapping:
 * - ALWAYS_ASK: shows a simple device credential prompt (password/PIN/pattern)
 * - BIOMETRIC: BiometricPrompt with BIOMETRIC_STRONG only
 * - PASSWORD: device credential only (password/PIN/pattern)
 * - BIOMETRIC_PASSWORD: BiometricPrompt with BIOMETRIC_STRONG | DEVICE_CREDENTIAL
 * - AUTO_APPROVE: skips all prompts (caller is responsible for showing warning on enable)
 */
@Singleton
class BiometricHelper @Inject constructor(
    private val biometricManager: BiometricManager,
) {
    /**
     * Authenticate according to the given confirmation policy.
     *
     * @param activity The FragmentActivity required by BiometricPrompt
     * @param policy The confirmation policy to enforce
     * @param title Title for the prompt dialog
     * @param subtitle Subtitle for the prompt dialog
     * @param callback Called with the authentication result
     */
    fun authenticate(
        activity: FragmentActivity,
        policy: ConfirmationPolicy,
        title: String,
        subtitle: String = "",
        callback: (AuthResult) -> Unit,
    ) {
        when (policy) {
            ConfirmationPolicy.AUTO_APPROVE -> {
                Timber.w("Auto-approve policy used for: %s", title)
                callback(AuthResult.Success)
            }
            ConfirmationPolicy.ALWAYS_ASK -> {
                showPrompt(
                    activity = activity,
                    authenticators = Authenticators.DEVICE_CREDENTIAL,
                    title = title,
                    subtitle = subtitle,
                    callback = callback,
                )
            }
            ConfirmationPolicy.BIOMETRIC -> {
                showPrompt(
                    activity = activity,
                    authenticators = Authenticators.BIOMETRIC_STRONG,
                    title = title,
                    subtitle = subtitle,
                    negativeButtonText = "Cancel",
                    callback = callback,
                )
            }
            ConfirmationPolicy.PASSWORD -> {
                showPrompt(
                    activity = activity,
                    authenticators = Authenticators.DEVICE_CREDENTIAL,
                    title = title,
                    subtitle = subtitle,
                    callback = callback,
                )
            }
            ConfirmationPolicy.BIOMETRIC_PASSWORD -> {
                showPrompt(
                    activity = activity,
                    authenticators = Authenticators.BIOMETRIC_STRONG or Authenticators.DEVICE_CREDENTIAL,
                    title = title,
                    subtitle = subtitle,
                    callback = callback,
                )
            }
        }
    }

    /**
     * Check whether the device can authenticate with the given policy.
     *
     * @return BiometricManager.BIOMETRIC_SUCCESS if the authenticator is available,
     *         or an error code from BiometricManager otherwise.
     */
    fun canAuthenticate(policy: ConfirmationPolicy): Int {
        return when (policy) {
            ConfirmationPolicy.AUTO_APPROVE -> BiometricManager.BIOMETRIC_SUCCESS
            ConfirmationPolicy.ALWAYS_ASK ->
                biometricManager.canAuthenticate(Authenticators.DEVICE_CREDENTIAL)
            ConfirmationPolicy.BIOMETRIC ->
                biometricManager.canAuthenticate(Authenticators.BIOMETRIC_STRONG)
            ConfirmationPolicy.PASSWORD ->
                biometricManager.canAuthenticate(Authenticators.DEVICE_CREDENTIAL)
            ConfirmationPolicy.BIOMETRIC_PASSWORD ->
                biometricManager.canAuthenticate(
                    Authenticators.BIOMETRIC_STRONG or Authenticators.DEVICE_CREDENTIAL,
                )
        }
    }

    /**
     * Authenticate according to the given unlock policy.
     *
     * @param activity The FragmentActivity required by BiometricPrompt
     * @param policy The unlock policy to enforce
     * @param title Title for the prompt dialog
     * @param subtitle Subtitle for the prompt dialog
     * @param callback Called with the authentication result
     */
    fun authenticateForUnlock(
        activity: FragmentActivity,
        policy: UnlockPolicy,
        title: String,
        subtitle: String = "",
        callback: (AuthResult) -> Unit,
    ) {
        when (policy) {
            UnlockPolicy.NONE -> {
                callback(AuthResult.Success)
            }
            UnlockPolicy.BIOMETRIC -> {
                showPrompt(
                    activity = activity,
                    authenticators = Authenticators.BIOMETRIC_STRONG,
                    title = title,
                    subtitle = subtitle,
                    negativeButtonText = "Cancel",
                    callback = callback,
                )
            }
            UnlockPolicy.PASSWORD -> {
                showPrompt(
                    activity = activity,
                    authenticators = Authenticators.DEVICE_CREDENTIAL,
                    title = title,
                    subtitle = subtitle,
                    callback = callback,
                )
            }
            UnlockPolicy.BIOMETRIC_PASSWORD -> {
                showPrompt(
                    activity = activity,
                    authenticators = Authenticators.BIOMETRIC_STRONG or Authenticators.DEVICE_CREDENTIAL,
                    title = title,
                    subtitle = subtitle,
                    callback = callback,
                )
            }
        }
    }

    /**
     * Returns true if auto-approve should show a security warning before being enabled.
     * Always returns true per FR-046.
     */
    fun requiresAutoApproveWarning(): Boolean = true

    /**
     * Returns true if [UnlockPolicy.NONE] should show a security warning before being enabled.
     * Always returns true per FR-046.
     */
    fun requiresNoneUnlockWarning(): Boolean = true

    private fun showPrompt(
        activity: FragmentActivity,
        authenticators: Int,
        title: String,
        subtitle: String,
        negativeButtonText: String? = null,
        callback: (AuthResult) -> Unit,
    ) {
        val executor = ContextCompat.getMainExecutor(activity)

        val authCallback = object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                Timber.i("Authentication succeeded for: %s", title)
                callback(AuthResult.Success)
            }

            override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                if (errorCode == BiometricPrompt.ERROR_USER_CANCELED ||
                    errorCode == BiometricPrompt.ERROR_NEGATIVE_BUTTON ||
                    errorCode == BiometricPrompt.ERROR_CANCELED
                ) {
                    Timber.i("Authentication cancelled for: %s", title)
                    callback(AuthResult.Cancelled)
                } else {
                    Timber.e("Authentication error for: %s code=%d msg=%s", title, errorCode, errString)
                    callback(AuthResult.Failure(errorCode, errString.toString()))
                }
            }

            override fun onAuthenticationFailed() {
                // Called on individual failed attempt (e.g., wrong fingerprint), not terminal.
                // BiometricPrompt keeps the dialog open for retry, so we don't callback here.
                Timber.w("Authentication attempt failed for: %s", title)
            }
        }

        val prompt = BiometricPrompt(activity, executor, authCallback)

        val builder = BiometricPrompt.PromptInfo.Builder()
            .setTitle(title)
            .setSubtitle(subtitle.ifEmpty { null })
            .setAllowedAuthenticators(authenticators)

        // setNegativeButtonText is required when NOT using DEVICE_CREDENTIAL,
        // and must NOT be set when DEVICE_CREDENTIAL is included
        if (authenticators and Authenticators.DEVICE_CREDENTIAL == 0 && negativeButtonText != null) {
            builder.setNegativeButtonText(negativeButtonText)
        }

        prompt.authenticate(builder.build())
    }
}
