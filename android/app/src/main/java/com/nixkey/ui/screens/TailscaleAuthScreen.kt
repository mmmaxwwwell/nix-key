package com.nixkey.ui.screens

import android.app.Activity
import android.content.Intent
import android.net.Uri
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import com.nixkey.R
import com.nixkey.tailscale.TailnetConnectionState
import com.nixkey.ui.components.LocalTailnetConnectionState
import com.nixkey.ui.viewmodel.TailscaleAuthPhase
import com.nixkey.ui.viewmodel.TailscaleAuthState
import com.nixkey.ui.viewmodel.TailscaleAuthViewModel

@Composable
fun TailscaleAuthScreen(onAuthSuccess: () -> Unit, viewModel: TailscaleAuthViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsState()

    LaunchedEffect(state.phase) {
        if (state.phase == TailscaleAuthPhase.SUCCESS) {
            onAuthSuccess()
        }
    }

    TailscaleAuthContent(
        state = state,
        onAuthKeyChanged = viewModel::onAuthKeyChanged,
        onConnectWithKey = viewModel::connectWithAuthKey,
        onConnectWithOAuth = viewModel::connectWithOAuth,
        onOAuthComplete = viewModel::onOAuthComplete,
        onRetry = viewModel::retry
    )
}

@Composable
fun TailscaleAuthContent(
    state: TailscaleAuthState,
    onAuthKeyChanged: (String) -> Unit,
    onConnectWithKey: () -> Unit,
    onConnectWithOAuth: () -> Unit,
    onOAuthComplete: () -> Unit,
    onRetry: () -> Unit
) {
    val context = LocalContext.current
    val tailnetState by LocalTailnetConnectionState.current.collectAsState()

    BackHandler {
        (context as? Activity)?.finishAffinity()
    }

    Scaffold { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center
        ) {
            // Tailnet connection indicator at the top
            TailscaleAuthIndicator(tailnetState)
            Spacer(modifier = Modifier.height(24.dp))

            // App logo
            Image(
                painter = painterResource(id = R.mipmap.ic_launcher),
                contentDescription = "nix-key logo",
                modifier = Modifier.size(72.dp)
            )
            Spacer(modifier = Modifier.height(16.dp))

            Text(
                text = "Connect to Tailscale",
                style = MaterialTheme.typography.headlineMedium
            )
            Spacer(modifier = Modifier.height(8.dp))
            Text(
                text = "Join your Tailnet to enable secure communication with your NixOS host.",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center
            )
            Spacer(modifier = Modifier.height(32.dp))

            when (state.phase) {
                TailscaleAuthPhase.INPUT -> {
                    OutlinedTextField(
                        value = state.authKey,
                        onValueChange = onAuthKeyChanged,
                        label = { Text("Auth Key") },
                        placeholder = { Text("tskey-auth-...") },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth(),
                        isError = state.error != null,
                        supportingText = if (state.error != null) {
                            {
                                Text(
                                    text = state.error,
                                    modifier = Modifier.semantics {
                                        liveRegion = LiveRegionMode.Polite
                                    }
                                )
                            }
                        } else {
                            null
                        }
                    )
                    Spacer(modifier = Modifier.height(16.dp))
                    Button(
                        onClick = onConnectWithKey,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text("Connect")
                    }
                    Spacer(modifier = Modifier.height(12.dp))
                    Text(
                        text = "or",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    Spacer(modifier = Modifier.height(12.dp))
                    OutlinedButton(
                        onClick = onConnectWithOAuth,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text("Sign in with Tailscale")
                    }
                }

                TailscaleAuthPhase.CONNECTING -> {
                    CircularProgressIndicator(modifier = Modifier.size(48.dp))
                    Spacer(modifier = Modifier.height(16.dp))
                    Text(
                        text = "Connecting to Tailnet...",
                        style = MaterialTheme.typography.bodyLarge
                    )
                }

                TailscaleAuthPhase.OAUTH_REQUIRED -> {
                    val oauthUrl = state.oauthUrl
                    if (oauthUrl != null) {
                        LaunchedEffect(oauthUrl) {
                            val intent = Intent(Intent.ACTION_VIEW, Uri.parse(oauthUrl))
                            context.startActivity(intent)
                        }
                    }
                    Text(
                        text = "Complete sign-in in your browser, then tap below.",
                        style = MaterialTheme.typography.bodyLarge,
                        textAlign = TextAlign.Center
                    )
                    Spacer(modifier = Modifier.height(16.dp))
                    Button(
                        onClick = onOAuthComplete,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text("I've signed in")
                    }
                }

                TailscaleAuthPhase.ERROR -> {
                    val errorText = state.error ?: "An unknown error occurred"
                    Text(
                        text = errorText,
                        style = MaterialTheme.typography.bodyLarge,
                        color = MaterialTheme.colorScheme.error,
                        textAlign = TextAlign.Center,
                        modifier = Modifier.semantics {
                            contentDescription = errorText
                            liveRegion = LiveRegionMode.Polite
                        }
                    )
                    Spacer(modifier = Modifier.height(16.dp))
                    Button(
                        onClick = onRetry,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text("Retry")
                    }
                }

                TailscaleAuthPhase.SUCCESS -> {
                    // Navigation handled by LaunchedEffect above
                }
            }
        }
    }
}

@Composable
private fun TailscaleAuthIndicator(state: TailnetConnectionState) {
    val (color, label) = when (state) {
        TailnetConnectionState.CONNECTED -> Color(0xFF4CAF50) to "Connected to Tailnet"
        TailnetConnectionState.CONNECTING -> Color(0xFFFFC107) to "Connecting to Tailnet..."
        TailnetConnectionState.DISCONNECTED -> Color(0xFFF44336) to "Disconnected"
    }

    Row(
        verticalAlignment = Alignment.CenterVertically
    ) {
        Surface(
            shape = CircleShape,
            color = color,
            modifier = Modifier.size(8.dp)
        ) {}
        Spacer(modifier = Modifier.width(6.dp))
        Text(
            text = label,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        if (state == TailnetConnectionState.CONNECTING) {
            Spacer(modifier = Modifier.width(8.dp))
            CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp)
        }
    }
}
