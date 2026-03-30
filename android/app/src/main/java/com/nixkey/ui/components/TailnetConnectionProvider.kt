package com.nixkey.ui.components

import androidx.compose.runtime.compositionLocalOf
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.MutableStateFlow
import com.nixkey.tailscale.TailnetConnectionState

/**
 * CompositionLocal providing the Tailnet connection state flow.
 * Screens collect this to show the persistent connection indicator (FR-110).
 */
val LocalTailnetConnectionState = compositionLocalOf<StateFlow<TailnetConnectionState>> {
    MutableStateFlow(TailnetConnectionState.DISCONNECTED)
}
