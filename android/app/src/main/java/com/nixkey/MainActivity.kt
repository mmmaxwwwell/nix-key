package com.nixkey

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import com.nixkey.service.GrpcServerService
import com.nixkey.ui.NixKeyAppUi
import dagger.hilt.android.AndroidEntryPoint

@AndroidEntryPoint
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            NixKeyAppUi()
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
