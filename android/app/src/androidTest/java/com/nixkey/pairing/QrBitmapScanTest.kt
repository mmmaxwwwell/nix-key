package com.nixkey.pairing

import android.graphics.Bitmap
import android.graphics.Color
import android.util.Base64
import androidx.test.runner.AndroidJUnit4
import com.google.mlkit.vision.barcode.BarcodeScanning
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.common.InputImage
import com.google.zxing.BarcodeFormat
import com.google.zxing.qrcode.QRCodeWriter
import com.nixkey.ui.viewmodel.PairingViewModel
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Instrumented test for QR code decode-parse path (T-QR-01, SC-015).
 *
 * Generates a QR code bitmap with a known nix-key pairing payload using ZXing,
 * feeds it to ML Kit barcode scanner via InputImage.fromBitmap(), and verifies
 * the full decode-parse pipeline without requiring a camera.
 */
@RunWith(AndroidJUnit4::class)
class QrBitmapScanTest {

    @Test
    fun mlKitScansZxingQr_basicPayload_decodesCorrectly() {
        val host = "100.64.0.1"
        val port = 12345
        val cert = "-----BEGIN CERTIFICATE-----\nMIIBkTCB+wIUYz123TEST\n-----END CERTIFICATE-----"
        val token = "pairing-token-abc"

        val payloadB64 = createQrPayload(host, port, cert, token, otel = null)
        val bitmap = generateQrBitmap(payloadB64)

        val rawValue = scanBitmapWithMlKit(bitmap)

        assertNotNull("ML Kit should detect a barcode", rawValue)
        assertEquals(payloadB64.trim(), rawValue!!.trim())

        val decoded = PairingViewModel.decodeQrPayload(rawValue)
        assertEquals(1, decoded.v)
        assertEquals(host, decoded.host)
        assertEquals(port, decoded.port)
        assertEquals(cert, decoded.cert)
        assertEquals(token, decoded.token)
        assertEquals(null, decoded.otel)
    }

    @Test
    fun mlKitScansZxingQr_withOtel_decodesCorrectly() {
        val host = "100.64.0.2"
        val port = 54321
        val cert = "-----BEGIN CERTIFICATE-----\nTESTCERT\n-----END CERTIFICATE-----"
        val token = "otel-token-xyz"
        val otel = "100.64.0.99:4317"

        val payloadB64 = createQrPayload(host, port, cert, token, otel)
        val bitmap = generateQrBitmap(payloadB64)

        val rawValue = scanBitmapWithMlKit(bitmap)

        assertNotNull("ML Kit should detect a barcode", rawValue)

        val decoded = PairingViewModel.decodeQrPayload(rawValue!!)
        assertEquals(1, decoded.v)
        assertEquals(host, decoded.host)
        assertEquals(port, decoded.port)
        assertEquals(cert, decoded.cert)
        assertEquals(token, decoded.token)
        assertEquals(otel, decoded.otel)
    }

    @Test
    fun mlKitScansZxingQr_largeCert_decodesCorrectly() {
        val host = "100.64.0.3"
        val port = 8443
        val cert = "-----BEGIN CERTIFICATE-----\n" +
            "MIICpDCCAYwCCQDU+pQ4pHgSpDANBgkqhkiG9w0BAQsFADAUMRIwEAYDVQQDDAls\n" +
            "b2NhbGhvc3QwHhcNMjQwMTAxMDAwMDAwWhcNMjUwMTAxMDAwMDAwWjAUMRIwEAYD\n" +
            "VQQDDAlsb2NhbGhvc3QwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQC7\n" +
            "-----END CERTIFICATE-----"
        val token = "large-cert-token"

        val payloadB64 = createQrPayload(host, port, cert, token, otel = null)
        val bitmap = generateQrBitmap(payloadB64, size = 800)

        val rawValue = scanBitmapWithMlKit(bitmap)

        assertNotNull("ML Kit should decode large QR payload", rawValue)

        val decoded = PairingViewModel.decodeQrPayload(rawValue!!)
        assertEquals(host, decoded.host)
        assertEquals(port, decoded.port)
        assertEquals(cert, decoded.cert)
        assertEquals(token, decoded.token)
    }

    private fun createQrPayload(
        host: String,
        port: Int,
        cert: String,
        token: String,
        otel: String?,
    ): String {
        val json = JSONObject().apply {
            put("v", 1)
            put("host", host)
            put("port", port)
            put("cert", cert)
            put("token", token)
            if (otel != null) put("otel", otel)
        }
        return Base64.encodeToString(json.toString().toByteArray(), Base64.NO_WRAP)
    }

    private fun generateQrBitmap(content: String, size: Int = 512): Bitmap {
        val writer = QRCodeWriter()
        val bitMatrix = writer.encode(content, BarcodeFormat.QR_CODE, size, size)
        val bitmap = Bitmap.createBitmap(size, size, Bitmap.Config.ARGB_8888)
        for (x in 0 until size) {
            for (y in 0 until size) {
                bitmap.setPixel(x, y, if (bitMatrix[x, y]) Color.BLACK else Color.WHITE)
            }
        }
        return bitmap
    }

    private fun scanBitmapWithMlKit(bitmap: Bitmap): String? {
        val inputImage = InputImage.fromBitmap(bitmap, 0)
        val scanner = BarcodeScanning.getClient()

        val latch = CountDownLatch(1)
        var result: String? = null
        var error: Exception? = null

        scanner.process(inputImage)
            .addOnSuccessListener { barcodes ->
                for (barcode in barcodes) {
                    if (barcode.valueType == Barcode.TYPE_TEXT) {
                        result = barcode.rawValue
                        break
                    }
                }
                latch.countDown()
            }
            .addOnFailureListener { e ->
                error = e
                latch.countDown()
            }

        assertTrue("ML Kit scan timed out", latch.await(10, TimeUnit.SECONDS))
        if (error != null) {
            fail("ML Kit scan failed: ${error!!.message}")
        }
        return result
    }
}
