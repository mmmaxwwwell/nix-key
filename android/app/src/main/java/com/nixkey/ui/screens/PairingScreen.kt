package com.nixkey.ui.screens

import android.Manifest
import android.content.pm.PackageManager
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.TextButton
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.google.mlkit.vision.barcode.BarcodeScanning
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.common.InputImage
import com.nixkey.ui.components.LocalTailnetConnectionState
import com.nixkey.ui.components.TailnetIndicator
import com.nixkey.ui.viewmodel.PairingPhase
import com.nixkey.ui.viewmodel.PairingViewModel
import java.util.concurrent.Executors

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PairingScreen(
    onBack: () -> Unit,
    onPairingComplete: () -> Unit,
    initialPayload: String? = null,
    viewModel: PairingViewModel = hiltViewModel()
) {
    val state by viewModel.state.collectAsState()
    val tailnetState by LocalTailnetConnectionState.current.collectAsState()

    LaunchedEffect(initialPayload) {
        if (initialPayload != null && state.phase == PairingPhase.SCANNING) {
            viewModel.onQrScanned(initialPayload)
        }
    }

    LaunchedEffect(state.phase) {
        if (state.phase == PairingPhase.SUCCESS) {
            onPairingComplete()
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Pair with Host") },
                navigationIcon = {
                    if (state.phase != PairingPhase.ERROR) {
                        TextButton(onClick = {
                            viewModel.resetState()
                            onBack()
                        }) {
                            Text("Cancel")
                        }
                    }
                },
                actions = {
                    TailnetIndicator(state = tailnetState)
                }
            )
        }
    ) { padding ->
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
        ) {
            when (state.phase) {
                PairingPhase.SCANNING -> {
                    QrScannerContent(
                        onQrScanned = { viewModel.onQrScanned(it) }
                    )
                }
                PairingPhase.CONFIRM_HOST -> {
                    ConfirmHostDialog(
                        hostName = state.payload?.host ?: "",
                        onConfirm = { viewModel.onHostConfirmed() },
                        onDeny = { viewModel.onHostDenied() }
                    )
                }
                PairingPhase.CONFIRM_OTEL -> {
                    ConfirmOtelDialog(
                        otelEndpoint = state.payload?.otel ?: "",
                        onAccept = { viewModel.onOtelAccepted() },
                        onDeny = { viewModel.onOtelDenied() }
                    )
                }
                PairingPhase.PAIRING -> {
                    PairingProgressContent(statusText = state.pairingStatusText)
                }
                PairingPhase.SUCCESS -> {
                    // LaunchedEffect above handles navigation
                }
                PairingPhase.ERROR -> {
                    ErrorContent(
                        error = state.error ?: "Unknown error",
                        onDone = {
                            viewModel.resetState()
                            onBack()
                        }
                    )
                }
            }
        }
    }
}

@Composable
fun QrScannerContent(onQrScanned: (String) -> Unit) {
    val context = LocalContext.current
    var hasCameraPermission by remember {
        mutableStateOf(
            ContextCompat.checkSelfPermission(context, Manifest.permission.CAMERA)
                == PackageManager.PERMISSION_GRANTED
        )
    }

    val permissionLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { granted ->
        hasCameraPermission = granted
    }

    LaunchedEffect(Unit) {
        if (!hasCameraPermission) {
            permissionLauncher.launch(Manifest.permission.CAMERA)
        }
    }

    if (hasCameraPermission) {
        CameraPreviewWithScanner(onQrScanned = onQrScanned)
    } else {
        Box(
            modifier = Modifier.fillMaxSize(),
            contentAlignment = Alignment.Center
        ) {
            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                Text("Camera permission is required to scan QR codes")
                Spacer(modifier = Modifier.height(16.dp))
                Button(onClick = { permissionLauncher.launch(Manifest.permission.CAMERA) }) {
                    Text("Grant Permission")
                }
            }
        }
    }
}

@Composable
@androidx.annotation.OptIn(androidx.camera.core.ExperimentalGetImage::class)
fun CameraPreviewWithScanner(onQrScanned: (String) -> Unit) {
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current
    var scanned by remember { mutableStateOf(false) }

    Box(modifier = Modifier.fillMaxSize()) {
        AndroidView(
            factory = { ctx ->
                val previewView = PreviewView(ctx)
                val cameraProviderFuture = ProcessCameraProvider.getInstance(ctx)

                cameraProviderFuture.addListener({
                    val cameraProvider = cameraProviderFuture.get()

                    val preview = Preview.Builder().build().also {
                        it.surfaceProvider = previewView.surfaceProvider
                    }

                    val barcodeScanner = BarcodeScanning.getClient()
                    val analysisExecutor = Executors.newSingleThreadExecutor()

                    val imageAnalysis = ImageAnalysis.Builder()
                        .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                        .build()
                        .also { analysis ->
                            analysis.setAnalyzer(analysisExecutor) { imageProxy ->
                                val mediaImage = imageProxy.image
                                if (mediaImage != null && !scanned) {
                                    val inputImage = InputImage.fromMediaImage(
                                        mediaImage,
                                        imageProxy.imageInfo.rotationDegrees
                                    )
                                    barcodeScanner.process(inputImage)
                                        .addOnSuccessListener { barcodes ->
                                            for (barcode in barcodes) {
                                                if (barcode.valueType == Barcode.TYPE_TEXT) {
                                                    val raw = barcode.rawValue
                                                    if (raw != null && !scanned) {
                                                        scanned = true
                                                        onQrScanned(raw)
                                                    }
                                                }
                                            }
                                        }
                                        .addOnCompleteListener {
                                            imageProxy.close()
                                        }
                                } else {
                                    imageProxy.close()
                                }
                            }
                        }

                    try {
                        cameraProvider.unbindAll()
                        cameraProvider.bindToLifecycle(
                            lifecycleOwner,
                            CameraSelector.DEFAULT_BACK_CAMERA,
                            preview,
                            imageAnalysis
                        )
                    } catch (e: Exception) {
                        // Camera binding can fail on devices without a camera
                    }
                }, ContextCompat.getMainExecutor(ctx))

                previewView
            },
            modifier = Modifier.fillMaxSize()
        )

        Column(
            modifier = Modifier
                .align(Alignment.BottomCenter)
                .padding(32.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Text(
                text = "Scanning...",
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.onSurface
            )
            Spacer(modifier = Modifier.height(4.dp))
            Text(
                text = "Point camera at the QR code",
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurface
            )
        }
    }
}

@Composable
fun ConfirmHostDialog(hostName: String, onConfirm: () -> Unit, onDeny: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDeny,
        title = { Text("Pair with host?") },
        text = {
            Text("Connect to $hostName?")
        },
        confirmButton = {
            Button(onClick = onConfirm) {
                Text("Accept")
            }
        },
        dismissButton = {
            OutlinedButton(onClick = onDeny) {
                Text("Deny")
            }
        }
    )
}

@Composable
fun ConfirmOtelDialog(otelEndpoint: String, onAccept: () -> Unit, onDeny: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDeny,
        title = { Text("Enable tracing?") },
        text = {
            Text("Traces will be sent to $otelEndpoint")
        },
        confirmButton = {
            Button(onClick = onAccept) {
                Text("Accept")
            }
        },
        dismissButton = {
            OutlinedButton(onClick = onDeny) {
                Text("Deny")
            }
        }
    )
}

@Composable
private fun PairingProgressContent(statusText: String) {
    Box(
        modifier = Modifier.fillMaxSize(),
        contentAlignment = Alignment.Center
    ) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            CircularProgressIndicator(modifier = Modifier.size(48.dp))
            Spacer(modifier = Modifier.height(16.dp))
            Text(
                text = statusText.ifEmpty { "Pairing with host..." },
                style = MaterialTheme.typography.bodyLarge
            )
        }
    }
}

@Composable
private fun ErrorContent(error: String, onDone: () -> Unit) {
    Box(
        modifier = Modifier.fillMaxSize(),
        contentAlignment = Alignment.Center
    ) {
        Column(
            horizontalAlignment = Alignment.CenterHorizontally,
            modifier = Modifier.padding(32.dp)
        ) {
            Text(
                text = "Pairing Failed",
                style = MaterialTheme.typography.titleLarge,
                color = MaterialTheme.colorScheme.error,
                modifier = Modifier.semantics {
                    heading()
                    contentDescription = "Pairing Failed"
                    liveRegion = LiveRegionMode.Polite
                }
            )
            Spacer(modifier = Modifier.height(8.dp))
            Text(
                text = error,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.semantics {
                    contentDescription = error
                    liveRegion = LiveRegionMode.Polite
                }
            )
            Spacer(modifier = Modifier.height(24.dp))
            Button(onClick = onDone) {
                Text("Done")
            }
        }
    }
}
