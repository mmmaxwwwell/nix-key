package com.nixkey.ui.screens

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
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.TextButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import com.nixkey.keystore.ConfirmationPolicy
import com.nixkey.keystore.UnlockPolicy
import com.nixkey.ui.components.LocalTailnetConnectionState
import com.nixkey.ui.components.TailnetIndicator
import com.nixkey.ui.viewmodel.SettingsViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    onBack: () -> Unit,
    onReauthenticate: () -> Unit = {},
    onLicenses: () -> Unit = {},
    viewModel: SettingsViewModel = hiltViewModel()
) {
    val state by viewModel.state.collectAsState()
    val tailnetState by LocalTailnetConnectionState.current.collectAsState()
    val context = LocalContext.current

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Settings") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                },
                actions = {
                    TailnetIndicator(state = tailnetState)
                }
            )
        }
    ) { padding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(16.dp)
                .verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            // Security section
            Text(
                text = "Security",
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.primary
            )

            SettingsToggle(
                title = "Allow key listing",
                description = "When off, all ListKeys requests return an empty list",
                checked = state.allowKeyListing,
                onCheckedChange = viewModel::setAllowKeyListing
            )

            DefaultUnlockPolicyPicker(
                selected = state.defaultUnlockPolicy,
                onSelected = viewModel::setDefaultUnlockPolicy
            )

            DefaultSigningPolicyPicker(
                selected = state.defaultConfirmationPolicy,
                onSelected = viewModel::setDefaultConfirmationPolicy
            )

            Spacer(modifier = Modifier.height(8.dp))
            HorizontalDivider()
            Spacer(modifier = Modifier.height(8.dp))

            // Tailscale section
            Text(
                text = "Tailscale",
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.primary
            )

            ReadOnlyField(label = "Tailscale IP", value = state.tailscaleIp.ifEmpty { "Not connected" })
            ReadOnlyField(label = "Tailnet name", value = state.tailnetName.ifEmpty { "Unknown" })

            OutlinedButton(
                onClick = { viewModel.onReauthenticate(onReauthenticate) },
                modifier = Modifier.fillMaxWidth()
            ) {
                Text("Re-authenticate")
            }

            Spacer(modifier = Modifier.height(8.dp))
            HorizontalDivider()
            Spacer(modifier = Modifier.height(8.dp))

            // Tracing section
            Text(
                text = "Tracing",
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.primary
            )

            SettingsToggle(
                title = "Enable tracing",
                description = "Export OpenTelemetry traces via OTLP",
                checked = state.otelEnabled,
                onCheckedChange = viewModel::setOtelEnabled
            )

            if (state.otelEnabled) {
                var hasFocus by remember { mutableStateOf(false) }
                OutlinedTextField(
                    value = state.otelEndpoint,
                    onValueChange = viewModel::setOtelEndpoint,
                    label = { Text("OTEL endpoint") },
                    placeholder = { Text("host:port") },
                    modifier = Modifier
                        .fillMaxWidth()
                        .onFocusChanged { focusState ->
                            if (hasFocus && !focusState.isFocused) {
                                viewModel.validateOtelEndpoint()
                            }
                            hasFocus = focusState.isFocused
                        },
                    singleLine = true,
                    isError = state.otelEndpointError != null,
                    supportingText = if (state.otelEndpointError != null) {
                        { Text(state.otelEndpointError!!) }
                    } else {
                        null
                    }
                )
            }

            Spacer(modifier = Modifier.height(8.dp))
            HorizontalDivider()
            Spacer(modifier = Modifier.height(8.dp))

            // About section
            Text(
                text = "About",
                style = MaterialTheme.typography.titleSmall,
                color = MaterialTheme.colorScheme.primary
            )

            val packageInfo = try {
                context.packageManager.getPackageInfo(context.packageName, 0)
            } catch (_: Exception) {
                null
            }

            ReadOnlyField(label = "App version", value = packageInfo?.versionName ?: "Unknown")
            ReadOnlyField(label = "Build info", value = "Build ${packageInfo?.longVersionCode ?: "Unknown"}")

            TextButton(onClick = onLicenses) {
                Text(
                    text = "Open source licenses",
                    style = MaterialTheme.typography.bodyLarge,
                    color = MaterialTheme.colorScheme.primary
                )
            }
        }
    }
}

@Composable
private fun ReadOnlyField(label: String, value: String) {
    Column {
        Text(
            text = label,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        Text(
            text = value,
            style = MaterialTheme.typography.bodyLarge
        )
    }
}

@Composable
private fun SettingsToggle(title: String, description: String, checked: Boolean, onCheckedChange: (Boolean) -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(text = title, style = MaterialTheme.typography.bodyLarge)
            Text(
                text = description,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
        Switch(checked = checked, onCheckedChange = onCheckedChange)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DefaultUnlockPolicyPicker(selected: UnlockPolicy, onSelected: (UnlockPolicy) -> Unit) {
    var expanded by remember { mutableStateOf(false) }

    Text("Default unlock policy", style = MaterialTheme.typography.bodyLarge)
    Text(
        text = "Applied to newly created keys",
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant
    )

    ExposedDropdownMenuBox(
        expanded = expanded,
        onExpandedChange = { expanded = !expanded }
    ) {
        OutlinedTextField(
            value = selected.settingsLabel(),
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
                    text = { Text(policy.settingsLabel()) },
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
private fun DefaultSigningPolicyPicker(selected: ConfirmationPolicy, onSelected: (ConfirmationPolicy) -> Unit) {
    var expanded by remember { mutableStateOf(false) }

    Text("Default signing policy", style = MaterialTheme.typography.bodyLarge)
    Text(
        text = "Applied to newly created keys",
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant
    )

    ExposedDropdownMenuBox(
        expanded = expanded,
        onExpandedChange = { expanded = !expanded }
    ) {
        OutlinedTextField(
            value = selected.settingsLabel(),
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
                    text = { Text(policy.settingsLabel()) },
                    onClick = {
                        onSelected(policy)
                        expanded = false
                    }
                )
            }
        }
    }
}

private fun UnlockPolicy.settingsLabel(): String = when (this) {
    UnlockPolicy.NONE -> "None"
    UnlockPolicy.BIOMETRIC -> "Biometric"
    UnlockPolicy.PASSWORD -> "Password"
    UnlockPolicy.BIOMETRIC_PASSWORD -> "Biometric + Password"
}

private fun ConfirmationPolicy.settingsLabel(): String = when (this) {
    ConfirmationPolicy.ALWAYS_ASK -> "Always ask"
    ConfirmationPolicy.BIOMETRIC -> "Biometric"
    ConfirmationPolicy.PASSWORD -> "Password"
    ConfirmationPolicy.BIOMETRIC_PASSWORD -> "Biometric + Password"
    ConfirmationPolicy.AUTO_APPROVE -> "Auto-approve"
}
