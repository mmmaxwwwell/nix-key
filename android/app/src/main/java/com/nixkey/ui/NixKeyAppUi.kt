package com.nixkey.ui

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.Modifier
import com.nixkey.tailscale.TailnetConnectionState
import com.nixkey.ui.components.LocalTailnetConnectionState
import com.nixkey.ui.navigation.NixKeyNavGraph
import com.nixkey.ui.theme.NixKeyTheme
import kotlinx.coroutines.flow.StateFlow

@Composable
fun NixKeyAppUi(
    needsTailscaleAuth: Boolean = false,
    deepLinkPayload: String? = null,
    onDeepLinkConsumed: () -> Unit = {},
    tailnetConnectionState: StateFlow<TailnetConnectionState>? = null
) {
    NixKeyTheme {
        val content: @Composable () -> Unit = {
            Surface(
                modifier = Modifier.fillMaxSize(),
                color = MaterialTheme.colorScheme.background
            ) {
                NixKeyNavGraph(
                    needsTailscaleAuth = needsTailscaleAuth,
                    deepLinkPayload = deepLinkPayload,
                    onDeepLinkConsumed = onDeepLinkConsumed
                )
            }
        }
        if (tailnetConnectionState != null) {
            CompositionLocalProvider(
                LocalTailnetConnectionState provides tailnetConnectionState
            ) {
                content()
            }
        } else {
            content()
        }
    }
}
