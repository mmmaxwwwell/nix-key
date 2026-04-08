package com.nixkey

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import com.nixkey.bridge.GoPhoneServer
import com.nixkey.keystore.AuthResult
import com.nixkey.keystore.BiometricHelper
import com.nixkey.keystore.ConfirmationPolicy
import com.nixkey.keystore.KeyManager
import com.nixkey.keystore.KeyUnlockManager
import com.nixkey.keystore.SignRequest
import com.nixkey.keystore.SignRequestQueue
import com.nixkey.keystore.SignRequestStatus
import com.nixkey.service.GrpcServerService
import com.nixkey.tailscale.TailscaleManager
import com.nixkey.ui.NixKeyAppUi
import com.nixkey.ui.screens.SignRequestDialog
import com.nixkey.ui.theme.NixKeyTheme
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import timber.log.Timber

@AndroidEntryPoint
class MainActivity : androidx.fragment.app.FragmentActivity() {

    @Inject
    lateinit var tailscaleManager: TailscaleManager

    @Inject
    lateinit var signRequestQueue: SignRequestQueue

    @Inject
    lateinit var goPhoneServer: GoPhoneServer

    @Inject
    lateinit var biometricHelper: BiometricHelper

    @Inject
    lateinit var keyManager: KeyManager

    @Inject
    lateinit var keyUnlockManager: KeyUnlockManager

    private var deepLinkPayload by mutableStateOf<String?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        if (savedInstanceState == null) {
            deepLinkPayload = extractPairPayload(intent)
            handleTestSignDeepLink(intent)
            // Clear intent data to prevent reprocessing on configuration change
            intent.data = null
        }
        val needsAuth = !tailscaleManager.hasStoredAuthKey() && !tailscaleManager.isRunning()

        // Eager unlock for keys with NONE unlock policy (FR-116)
        keyUnlockManager.eagerUnlockNoneKeys(keyManager.listKeys())

        setContent {
            NixKeyTheme {
                NixKeyAppUi(
                    needsTailscaleAuth = needsAuth,
                    deepLinkPayload = deepLinkPayload,
                    onDeepLinkConsumed = { deepLinkPayload = null },
                    tailnetConnectionState = tailscaleManager.connectionState
                )
                SignRequestDialog(
                    queue = signRequestQueue,
                    onApprove = { request -> handleApprove(request) },
                    onDeny = { request ->
                        signRequestQueue.complete(request.requestId, SignRequestStatus.DENIED)
                        goPhoneServer.confirmerAdapter.notifyCompletion(
                            request.requestId,
                            SignRequestStatus.DENIED
                        )
                    }
                )
            }
        }
    }

    /**
     * Handle approve for a sign request. If the key is locked, trigger unlock first,
     * then proceed to signing confirmation. If unlock fails, deny the request and
     * let queued requests retry their own unlock (FR-053).
     */
    private fun handleApprove(request: SignRequest) {
        if (request.needsUnlock && !keyUnlockManager.isUnlocked(request.keyFingerprint)) {
            // Step 1: Unlock the key first
            biometricHelper.authenticateForUnlock(
                activity = this@MainActivity,
                policy = request.unlockPolicy,
                title = "Unlock Key",
                subtitle = "Key: ${request.keyName} for ${request.hostName}"
            ) { unlockResult ->
                when (unlockResult) {
                    is AuthResult.Success -> {
                        // Mark key as unlocked
                        val keyInfo = keyManager.listKeys().find {
                            it.fingerprint == request.keyFingerprint
                        }
                        if (keyInfo != null) {
                            keyUnlockManager.unlock(keyInfo)
                        }
                        // Step 2: Now do signing confirmation
                        proceedWithSigningConfirmation(request)
                    }
                    else -> {
                        // Unlock failed or cancelled — deny this request
                        Timber.w("Unlock failed for request=%s", request.requestId)
                        signRequestQueue.complete(request.requestId, SignRequestStatus.DENIED)
                        goPhoneServer.confirmerAdapter.notifyCompletion(
                            request.requestId,
                            SignRequestStatus.DENIED
                        )
                    }
                }
            }
        } else {
            // Key already unlocked or doesn't need unlock
            proceedWithSigningConfirmation(request)
        }
    }

    private fun proceedWithSigningConfirmation(request: SignRequest) {
        biometricHelper.authenticate(
            activity = this@MainActivity,
            policy = request.confirmationPolicy,
            title = "Sign Request",
            subtitle = "Key: ${request.keyName} for ${request.hostName}"
        ) { result ->
            val status = when (result) {
                is AuthResult.Success -> SignRequestStatus.APPROVED
                else -> SignRequestStatus.DENIED
            }
            signRequestQueue.complete(request.requestId, status)
            goPhoneServer.confirmerAdapter.notifyCompletion(
                request.requestId,
                status
            )
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        extractPairPayload(intent)?.let { payload ->
            deepLinkPayload = payload
        }
        handleTestSignDeepLink(intent)
        // Clear intent data to prevent reprocessing on configuration change
        intent.data = null
        setIntent(Intent())
    }

    private var wasChangingConfigurations = false

    override fun onStart() {
        super.onStart()
        // Request POST_NOTIFICATIONS permission on Android 13+ (API 33+)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS)
                != PackageManager.PERMISSION_GRANTED
            ) {
                ActivityCompat.requestPermissions(
                    this,
                    arrayOf(Manifest.permission.POST_NOTIFICATIONS),
                    REQUEST_NOTIFICATION_PERMISSION
                )
            }
        }
        // Skip starting the service if we're resuming from a configuration change
        // (e.g. rotation) — the service was intentionally kept running
        if (!wasChangingConfigurations) {
            GrpcServerService.startService(this)
        }
        wasChangingConfigurations = false
    }

    override fun onStop() {
        super.onStop()
        // Don't stop the service during configuration changes (e.g. rotation)
        // to avoid a race condition where startForegroundService() in onStart()
        // fails to call startForeground() within the system's 10-second deadline
        wasChangingConfigurations = isChangingConfigurations
        if (!isChangingConfigurations) {
            GrpcServerService.stopService(this)
        }
    }

    /**
     * Handle the debug-only nix-key://test-sign deep link by injecting a fake
     * sign request into the queue. Only active in debug builds.
     */
    private fun handleTestSignDeepLink(intent: Intent?) {
        if (!BuildConfig.DEBUG) return
        val uri = intent?.data ?: return
        if (uri.scheme != "nix-key" || uri.host != "test-sign") return
        val policyParam = uri.getQueryParameter("policy")?.uppercase()
        val confirmationPolicy = if (policyParam != null) {
            try {
                ConfirmationPolicy.valueOf(policyParam)
            } catch (_: IllegalArgumentException) {
                ConfirmationPolicy.ALWAYS_ASK
            }
        } else {
            ConfirmationPolicy.ALWAYS_ASK
        }
        val request = SignRequest(
            keyFingerprint = uri.getQueryParameter("fingerprint") ?: "SHA256:e2e-test",
            hostName = uri.getQueryParameter("host") ?: "e2e-test-host",
            keyName = uri.getQueryParameter("key") ?: "e2e-test-key",
            dataToSign = "e2e-test-data-${System.currentTimeMillis()}".toByteArray(),
            confirmationPolicy = confirmationPolicy,
            needsUnlock = uri.getQueryParameter("unlock")?.toBoolean() ?: false
        )
        Timber.d("Test sign request injected: host=%s key=%s", request.hostName, request.keyName)
        signRequestQueue.enqueue(request)
    }

    companion object {
        private const val REQUEST_NOTIFICATION_PERMISSION = 1001

        fun extractPairPayload(intent: Intent?): String? {
            val uri = intent?.data ?: return null
            if (uri.scheme == "nix-key" && uri.host == "pair") {
                return uri.getQueryParameter("payload")
            }
            return null
        }
    }
}
