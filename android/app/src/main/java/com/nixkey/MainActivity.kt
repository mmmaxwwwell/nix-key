package com.nixkey

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import com.nixkey.service.GrpcServerService
import com.nixkey.tailscale.TailscaleManager
import com.nixkey.ui.NixKeyAppUi
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject

@AndroidEntryPoint
class MainActivity : ComponentActivity() {

    @Inject
    lateinit var tailscaleManager: TailscaleManager

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val needsAuth = !tailscaleManager.hasStoredAuthKey() && !tailscaleManager.isRunning()
        setContent {
            NixKeyAppUi(needsTailscaleAuth = needsAuth)
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
}
