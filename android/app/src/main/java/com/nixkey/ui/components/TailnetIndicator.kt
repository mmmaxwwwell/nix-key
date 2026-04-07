package com.nixkey.ui.components

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import com.nixkey.tailscale.TailnetConnectionState

/**
 * Persistent Tailnet connection indicator (FR-110).
 *
 * Displays a colored dot with label:
 * - Green / "Connected"
 * - Yellow / "Connecting"
 * - Red / "Disconnected"
 */
@Composable
fun TailnetIndicator(state: TailnetConnectionState, modifier: Modifier = Modifier) {
    val (color, label) = when (state) {
        TailnetConnectionState.CONNECTED -> Color(0xFF4CAF50) to "Connected"
        TailnetConnectionState.CONNECTING -> Color(0xFFFFC107) to "Connecting"
        TailnetConnectionState.DISCONNECTED -> Color(0xFFF44336) to "Disconnected"
    }

    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = modifier
            .padding(end = 8.dp)
            .semantics(mergeDescendants = true) {
                contentDescription = "Connection status: $label"
            }
    ) {
        Surface(
            shape = CircleShape,
            color = color,
            modifier = Modifier.size(8.dp)
        ) {}
        Spacer(modifier = Modifier.width(4.dp))
        Text(
            text = label,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}
