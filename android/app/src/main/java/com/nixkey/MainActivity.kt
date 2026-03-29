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
import com.nixkey.keystore.SignRequestQueue
import com.nixkey.keystore.SignRequestStatus
import com.nixkey.service.GrpcServerService
import com.nixkey.tailscale.TailscaleManager
import com.nixkey.ui.NixKeyAppUi
import com.nixkey.ui.screens.SignRequestDialog
import com.nixkey.ui.theme.NixKeyTheme
import dagger.hilt.android.AndroidEntryPoint
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

    private var deepLinkPayload by mutableStateOf<String?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        deepLinkPayload = extractPairPayload(intent)
        val needsAuth = !tailscaleManager.hasStoredAuthKey() && !tailscaleManager.isRunning()
        setContent {
            NixKeyTheme {
                NixKeyAppUi(
                    needsTailscaleAuth = needsAuth,
                    deepLinkPayload = deepLinkPayload,
                    onDeepLinkConsumed = { deepLinkPayload = null },
                )
                SignRequestDialog(
                    queue = signRequestQueue,
                    onApprove = { request ->
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
                    },
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
