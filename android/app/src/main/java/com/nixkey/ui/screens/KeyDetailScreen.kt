package com.nixkey.ui.screens

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import com.nixkey.keystore.ConfirmationPolicy
import com.nixkey.keystore.KeyType
import com.nixkey.keystore.UnlockPolicy
import com.nixkey.ui.components.LocalTailnetConnectionState
import com.nixkey.ui.components.TailnetIndicator
import com.nixkey.ui.viewmodel.KeyDetailViewModel
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun KeyDetailScreen(onBack: () -> Unit, viewModel: KeyDetailViewModel = hiltViewModel()) {
    val state by viewModel.state.collectAsState()
    val snackbarHostState = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current
    val tailnetState by LocalTailnetConnectionState.current.collectAsState()

    LaunchedEffect(state.keyDeleted) {
        if (state.keyDeleted) onBack()
    }

    if (state.showAutoApproveWarning) {
        AutoApproveWarningDialog(
            onConfirm = viewModel::confirmAutoApprove,
            onDismiss = viewModel::dismissAutoApproveWarning
        )
    }

    if (state.showNoneUnlockWarning) {
        NoneUnlockWarningDialog(
            onConfirm = viewModel::confirmNoneUnlock,
            onDismiss = viewModel::dismissNoneUnlockWarning
        )
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(if (state.isCreateMode) "Create Key" else state.displayName) },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                },
                actions = {
                    TailnetIndicator(state = tailnetState)
                }
            )
        },
        snackbarHost = { SnackbarHost(snackbarHostState) }
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            // Key name
            OutlinedTextField(
                value = state.displayName,
                onValueChange = viewModel::setDisplayName,
                label = { Text("Key name") },
                modifier = Modifier.fillMaxWidth(),
                singleLine = true,
                isError = state.error != null,
                supportingText = state.error?.let { error -> { Text(error) } }
            )

            // Key type
            if (state.isCreateMode) {
                Text("Key type", style = MaterialTheme.typography.labelLarge)
                Row(
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    KeyType.entries.forEach { type ->
                        FilterChip(
                            selected = state.keyType == type,
                            onClick = { viewModel.setKeyType(type) },
                            label = { Text(type.name.replace("_", "-")) }
                        )
                    }
                }
                Text(
                    text = when (state.keyType) {
                        KeyType.ED25519 -> "Software-generated, encrypted with Keystore wrapping key"
                        KeyType.ECDSA_P256 -> "Hardware-backed via Android Keystore (TEE/StrongBox)"
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            } else {
                // Read-only key type
                Text("Key type", style = MaterialTheme.typography.labelLarge)
                Text(
                    text = state.keyType.name.replace("_", "-"),
                    style = MaterialTheme.typography.bodyLarge
                )

                // Fingerprint
                Text("Fingerprint", style = MaterialTheme.typography.labelLarge)
                Text(
                    text = state.keyInfo?.fingerprint ?: "",
                    style = MaterialTheme.typography.bodyMedium
                )

                // Lock status
                Text("Lock status", style = MaterialTheme.typography.labelLarge)
                Text(
                    text = if (state.isUnlocked) "Unlocked" else "Locked",
                    style = MaterialTheme.typography.bodyMedium,
                    color = if (state.isUnlocked) {
                        MaterialTheme.colorScheme.primary
                    } else {
                        MaterialTheme.colorScheme.onSurfaceVariant
                    }
                )
            }

            // Unlock policy picker
            UnlockPolicyPicker(
                selected = state.unlockPolicy,
                onSelected = viewModel::setUnlockPolicy
            )

            // Signing policy picker
            SigningPolicyPicker(
                selected = state.confirmationPolicy,
                onSelected = viewModel::setConfirmationPolicy
            )

            if (state.isCreateMode) {
                Button(
                    onClick = viewModel::createKey,
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text("Create")
                }
            } else {
                // Export section
                Text("Export Public Key", style = MaterialTheme.typography.labelLarge)
                Row(
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    OutlinedButton(
                        onClick = {
                            copyToClipboard(context, state.publicKeyString)
                            scope.launch { snackbarHostState.showSnackbar("Copied to clipboard") }
                        }
                    ) {
                        Text("Copy")
                    }
                    OutlinedButton(
                        onClick = { sharePublicKey(context, state.publicKeyString) }
                    ) {
                        Text("Share")
                    }
                    OutlinedButton(
                        onClick = {
                            scope.launch { snackbarHostState.showSnackbar("QR code display not yet implemented") }
                        }
                    ) {
                        Text("QR Code")
                    }
                }

                // Save button
                if (state.hasUnsavedChanges) {
                    Button(
                        onClick = viewModel::saveChanges,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text("Save")
                    }
                }

                // Lock/Unlock button
                if (state.isUnlocked) {
                    OutlinedButton(
                        onClick = viewModel::lockKey,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text("Lock Key")
                    }
                }

                Spacer(modifier = Modifier.height(16.dp))

                // Delete button
                Button(
                    onClick = viewModel::deleteKey,
                    modifier = Modifier.fillMaxWidth(),
                    colors = ButtonDefaults.buttonColors(
                        containerColor = MaterialTheme.colorScheme.error
                    )
                ) {
                    Text("Delete Key")
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun UnlockPolicyPicker(selected: UnlockPolicy, onSelected: (UnlockPolicy) -> Unit) {
    var expanded by remember { mutableStateOf(false) }

    Text("Unlock policy", style = MaterialTheme.typography.labelLarge)

    ExposedDropdownMenuBox(
        expanded = expanded,
        onExpandedChange = { expanded = !expanded }
    ) {
        OutlinedTextField(
            value = selected.displayLabel(),
            onValueChange = {},
            readOnly = true,
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded) },
            modifier = Modifier
                .menuAnchor(MenuAnchorType.PrimaryNotEditable)
                .fillMaxWidth()
        )
        ExposedDropdownMenu(
            expanded = expanded,
            onDismissRequest = { expanded = false }
        ) {
            UnlockPolicy.entries.forEach { policy ->
                DropdownMenuItem(
                    text = { Text(policy.displayLabel()) },
                    onClick = {
                        onSelected(policy)
                        expanded = false
                    }
                )
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SigningPolicyPicker(selected: ConfirmationPolicy, onSelected: (ConfirmationPolicy) -> Unit) {
    var expanded by remember { mutableStateOf(false) }

    Text("Signing policy", style = MaterialTheme.typography.labelLarge)

    ExposedDropdownMenuBox(
        expanded = expanded,
        onExpandedChange = { expanded = !expanded }
    ) {
        OutlinedTextField(
            value = selected.displayLabel(),
            onValueChange = {},
            readOnly = true,
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = expanded) },
            modifier = Modifier
                .menuAnchor(MenuAnchorType.PrimaryNotEditable)
                .fillMaxWidth()
        )
        ExposedDropdownMenu(
            expanded = expanded,
            onDismissRequest = { expanded = false }
        ) {
            ConfirmationPolicy.entries.forEach { policy ->
                DropdownMenuItem(
                    text = { Text(policy.displayLabel()) },
                    onClick = {
                        onSelected(policy)
                        expanded = false
                    }
                )
            }
        }
    }
}

@Composable
private fun AutoApproveWarningDialog(onConfirm: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Security Warning") },
        text = {
            Text(
                "Auto-approve allows sign requests to be processed without your confirmation. " +
                    "Any host with a valid mTLS certificate can trigger signing operations silently. " +
                    "Are you sure?"
            )
        },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text("Enable Auto-Approve")
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text("Cancel")
            }
        }
    )
}

@Composable
private fun NoneUnlockWarningDialog(onConfirm: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Security Warning") },
        text = {
            Text(
                "Disabling unlock means key material will be decrypted automatically on app start " +
                    "without any authentication. Combined with auto-approve signing, this allows " +
                    "completely silent signing operations. Are you sure?"
            )
        },
        confirmButton = {
            TextButton(onClick = onConfirm) {
                Text("Disable Unlock")
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text("Cancel")
            }
        }
    )
}

private fun UnlockPolicy.displayLabel(): String = when (this) {
    UnlockPolicy.NONE -> "None"
    UnlockPolicy.BIOMETRIC -> "Biometric"
    UnlockPolicy.PASSWORD -> "Password"
    UnlockPolicy.BIOMETRIC_PASSWORD -> "Biometric + Password"
}

private fun ConfirmationPolicy.displayLabel(): String = when (this) {
    ConfirmationPolicy.ALWAYS_ASK -> "Always ask"
    ConfirmationPolicy.BIOMETRIC -> "Biometric"
    ConfirmationPolicy.PASSWORD -> "Password"
    ConfirmationPolicy.BIOMETRIC_PASSWORD -> "Biometric + Password"
    ConfirmationPolicy.AUTO_APPROVE -> "Auto-approve"
}

private fun copyToClipboard(context: Context, text: String) {
    val clipboard = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
    clipboard.setPrimaryClip(ClipData.newPlainText("SSH Public Key", text))
}

private fun sharePublicKey(context: Context, publicKey: String) {
    val intent = Intent(Intent.ACTION_SEND).apply {
        type = "text/plain"
        putExtra(Intent.EXTRA_TEXT, publicKey)
    }
    context.startActivity(Intent.createChooser(intent, "Share SSH Public Key"))
}
