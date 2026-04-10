package com.nixkey.ui.screens

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LicensesScreen(onBack: () -> Unit) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Open source licenses") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                }
            )
        }
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp)
                .verticalScroll(rememberScrollState())
        ) {
            licenses.forEach { license ->
                LicenseEntry(license)
                Spacer(modifier = Modifier.height(4.dp))
                HorizontalDivider()
                Spacer(modifier = Modifier.height(4.dp))
            }
        }
    }
}

@Composable
private fun LicenseEntry(license: LicenseInfo) {
    Column(
        modifier = Modifier.semantics {
            contentDescription = "${license.name}, ${license.license}"
        }
    ) {
        Text(text = license.name, style = MaterialTheme.typography.titleSmall)
        Text(
            text = license.license,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}

private data class LicenseInfo(val name: String, val license: String)

private val licenses = listOf(
    LicenseInfo("AndroidX Core", "Apache License 2.0"),
    LicenseInfo("AndroidX Activity Compose", "Apache License 2.0"),
    LicenseInfo("AndroidX Lifecycle", "Apache License 2.0"),
    LicenseInfo("AndroidX Biometric", "Apache License 2.0"),
    LicenseInfo("AndroidX Security Crypto", "Apache License 2.0"),
    LicenseInfo("Jetpack Compose", "Apache License 2.0"),
    LicenseInfo("Jetpack Compose Material 3", "Apache License 2.0"),
    LicenseInfo("Navigation Compose", "Apache License 2.0"),
    LicenseInfo("Hilt", "Apache License 2.0"),
    LicenseInfo("CameraX", "Apache License 2.0"),
    LicenseInfo("ML Kit Barcode Scanning", "Apache License 2.0"),
    LicenseInfo("gRPC", "Apache License 2.0"),
    LicenseInfo("Protocol Buffers", "BSD 3-Clause License"),
    LicenseInfo("Bouncy Castle", "MIT License"),
    LicenseInfo("Timber", "Apache License 2.0"),
    LicenseInfo("OkHttp", "Apache License 2.0")
)
