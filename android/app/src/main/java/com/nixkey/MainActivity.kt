package com.nixkey

import android.content.Intent
import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import com.nixkey.bridge.GoPhoneServer
import com.nixkey.keystore.AuthResult
import com.nixkey.keystore.BiometricHelper
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
import timber.log.Timber
import javax.inject.Inject

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
        deepLinkPayload = extractPairPayload(intent)
        val needsAuth = !tailscaleManager.hasStoredAuthKey() && !tailscaleManager.isRunning()

        // Eager unlock for keys with NONE unlock policy (FR-116)
        keyUnlockManager.eagerUnlockNoneKeys(keyManager.listKeys())

        setContent {
            NixKeyTheme {
                NixKeyAppUi(
                    needsTailscaleAuth = needsAuth,
                    deepLinkPayload = deepLinkPayload,
                    onDeepLinkConsumed = { deepLinkPayload = null },
                )
                SignRequestDialog(
                    queue = signRequestQueue,
                    onApprove = { request -> handleApprove(request) },
                    onDeny = { request ->
                        signRequestQueue.complete(request.requestId, SignRequestStatus.DENIED)
                        goPhoneServer.confirmerAdapter.notifyCompletion(
                            request.requestId,
                            SignRequestStatus.DENIED,
                        )
                    },
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
                subtitle = "Key: ${request.keyName} for ${request.hostName}",
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
                            SignRequestStatus.DENIED,
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
            subtitle = "Key: ${request.keyName} for ${request.hostName}",
        ) { result ->
            val status = when (result) {
                is AuthResult.Success -> SignRequestStatus.APPROVED
                else -> SignRequestStatus.DENIED
            }
            signRequestQueue.complete(request.requestId, status)
            goPhoneServer.confirmerAdapter.notifyCompletion(
                request.requestId,
                status,
            )
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        extractPairPayload(intent)?.let { payload ->
            deepLinkPayload = payload
        }
    }

    override fun onStart() {
        super.onStart()
        GrpcServerService.startService(this)
    }

    override fun onStop() {
        super.onStop()
        GrpcServerService.stopService(this)
    }

    companion object {
        fun extractPairPayload(intent: Intent?): String? {
            val uri = intent?.data ?: return null
            if (uri.scheme == "nix-key" && uri.host == "pair") {
                return uri.getQueryParameter("payload")
            }
            return null
        }
    }
}
