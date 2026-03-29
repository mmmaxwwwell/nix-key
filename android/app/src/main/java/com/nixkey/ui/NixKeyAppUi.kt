package com.nixkey.ui

import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import com.nixkey.ui.navigation.NixKeyNavGraph
import com.nixkey.ui.theme.NixKeyTheme

@Composable
fun NixKeyAppUi(
    needsTailscaleAuth: Boolean = false,
    deepLinkPayload: String? = null,
    onDeepLinkConsumed: () -> Unit = {},
) {
    NixKeyTheme {
        Surface(
            modifier = Modifier.fillMaxSize(),
            color = MaterialTheme.colorScheme.background,
        ) {
            NixKeyNavGraph(
                needsTailscaleAuth = needsTailscaleAuth,
                deepLinkPayload = deepLinkPayload,
                onDeepLinkConsumed = onDeepLinkConsumed,
            )
        }
    }
}
