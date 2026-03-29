package com.nixkey.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import com.nixkey.keystore.SignRequest
import com.nixkey.keystore.SignRequestQueue

/**
 * Compose dialog overlay that shows sign request details and Approve/Deny buttons.
 *
 * Observes the [SignRequestQueue] and shows the front-of-queue request. When the user
 * responds, the queue advances to the next request (if any).
 *
 * @param queue The sign request queue to observe
 * @param onApprove Called when the user approves a request (triggers BiometricHelper per key policy)
 * @param onDeny Called when the user denies a request
 */
@Composable
fun SignRequestDialog(
    queue: SignRequestQueue,
    onApprove: (SignRequest) -> Unit,
    onDeny: (SignRequest) -> Unit,
) {
    val currentRequest by queue.currentRequest.collectAsState()
    val queueSize by queue.queueSize.collectAsState()

    currentRequest?.let { request ->
        SignRequestDialogContent(
            request = request,
            queueSize = queueSize,
            onApprove = { onApprove(request) },
            onDeny = { onDeny(request) },
        )
    }
}

/**
 * The actual dialog content, separated for testability without needing a queue.
 */
@Composable
fun SignRequestDialogContent(
    request: SignRequest,
    queueSize: Int = 0,
    onApprove: () -> Unit,
    onDeny: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = { /* Sign requests cannot be dismissed by tapping outside */ },
        modifier = Modifier.testTag("sign_request_dialog"),
        title = {
            Text("Sign Request")
        },
        text = {
            Column(
                modifier = Modifier.fillMaxWidth(),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                DetailRow(label = "Host", value = request.hostName)
                DetailRow(label = "Key", value = request.keyName)
                DetailRow(
                    label = "Data hash",
                    value = request.dataHashTruncated(),
                    monospace = true,
                )
                if (queueSize > 0) {
                    Spacer(modifier = Modifier.height(4.dp))
                    Text(
                        text = "$queueSize more request${if (queueSize != 1) "s" else ""} waiting",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.testTag("queue_count"),
                    )
                }
            }
        },
        confirmButton = {
            Button(
                onClick = onApprove,
                modifier = Modifier.testTag("approve_button"),
            ) {
                Text("Approve")
            }
        },
        dismissButton = {
            OutlinedButton(
                onClick = onDeny,
                modifier = Modifier.testTag("deny_button"),
                colors = ButtonDefaults.outlinedButtonColors(
                    contentColor = MaterialTheme.colorScheme.error,
                ),
            ) {
                Text("Deny")
            }
        },
    )
}

@Composable
private fun DetailRow(
    label: String,
    value: String,
    monospace: Boolean = false,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 2.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(
            text = "$label:",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Text(
            text = value,
            style = MaterialTheme.typography.bodyMedium,
            fontFamily = if (monospace) FontFamily.Monospace else FontFamily.Default,
        )
    }
}
