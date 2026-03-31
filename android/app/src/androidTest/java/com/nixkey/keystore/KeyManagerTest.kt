package com.nixkey.keystore

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import androidx.test.runner.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.Signature
import java.security.interfaces.ECPublicKey
import java.util.Base64
import org.bouncycastle.crypto.params.Ed25519PublicKeyParameters
import org.bouncycastle.crypto.signers.Ed25519Signer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class KeyManagerTest {

    private lateinit var keyManager: KeyManager
    private val createdAliases = mutableListOf<String>()

    @Before
    fun setUp() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        keyManager = KeyManager(context)
    }

    @After
    fun tearDown() {
        // Clean up all keys created during tests
        for (alias in createdAliases) {
            try {
                keyManager.deleteKey(alias)
            } catch (_: Exception) {
                // Ignore cleanup errors
            }
        }
    }

    @Test
    fun createEcdsaKey_createsKeyInKeystore() {
        val info = keyManager.createKey("test-ecdsa", KeyType.ECDSA_P256)
        createdAliases.add(info.alias)

        assertEquals("test-ecdsa", info.displayName)
        assertEquals(KeyType.ECDSA_P256, info.keyType)
        assertTrue(info.fingerprint.startsWith("SHA256:"))
        assertEquals(ConfirmationPolicy.ALWAYS_ASK, info.confirmationPolicy)
        assertEquals(null, info.wrappingKeyAlias)

        // Verify key exists in Android Keystore
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        assertTrue(ks.containsAlias(info.alias))
    }

    @Test
    fun createEd25519Key_createsKeyWithWrappingKey() {
        val info = keyManager.createKey("test-ed25519", KeyType.ED25519)
        createdAliases.add(info.alias)

        assertEquals("test-ed25519", info.displayName)
        assertEquals(KeyType.ED25519, info.keyType)
        assertTrue(info.fingerprint.startsWith("SHA256:"))
        assertEquals(ConfirmationPolicy.ALWAYS_ASK, info.confirmationPolicy)
        assertNotNull(info.wrappingKeyAlias)

        // Verify wrapping key exists in Android Keystore
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        assertTrue(ks.containsAlias(info.wrappingKeyAlias))
    }

    @Test
    fun createKey_respectsConfirmationPolicy() {
        val info = keyManager.createKey(
            "test-policy",
            KeyType.ED25519,
            signingPolicy = ConfirmationPolicy.BIOMETRIC,
        )
        createdAliases.add(info.alias)

        assertEquals(ConfirmationPolicy.BIOMETRIC, info.confirmationPolicy)
    }

    @Test
    fun listKeys_returnsCreatedKeys() {
        val ecdsa = keyManager.createKey("ecdsa-list", KeyType.ECDSA_P256)
        val ed25519 = keyManager.createKey("ed25519-list", KeyType.ED25519)
        createdAliases.add(ecdsa.alias)
        createdAliases.add(ed25519.alias)

        val keys = keyManager.listKeys()
        val aliases = keys.map { it.alias }
        assertTrue(aliases.contains(ecdsa.alias))
        assertTrue(aliases.contains(ed25519.alias))
    }

    @Test
    fun exportPublicKey_ecdsaSshFormat() {
        val info = keyManager.createKey("ecdsa-export", KeyType.ECDSA_P256)
        createdAliases.add(info.alias)

        val sshKey = keyManager.exportPublicKey(info.alias)
        assertTrue(sshKey.startsWith("ecdsa-sha2-nistp256 "))
        assertTrue(sshKey.endsWith(" ecdsa-export"))

        // Verify base64 portion decodes without error
        val parts = sshKey.split(" ")
        assertEquals(3, parts.size)
        val blob = Base64.getDecoder().decode(parts[1])
        assertTrue(blob.isNotEmpty())
    }

    @Test
    fun exportPublicKey_ed25519SshFormat() {
        val info = keyManager.createKey("ed25519-export", KeyType.ED25519)
        createdAliases.add(info.alias)

        val sshKey = keyManager.exportPublicKey(info.alias)
        assertTrue(sshKey.startsWith("ssh-ed25519 "))
        assertTrue(sshKey.endsWith(" ed25519-export"))

        // Verify base64 portion decodes without error
        val parts = sshKey.split(" ")
        assertEquals(3, parts.size)
        val blob = Base64.getDecoder().decode(parts[1])
        assertTrue(blob.isNotEmpty())
    }

    @Test
    fun signEcdsa_producesVerifiableSignature() {
        val info = keyManager.createKey("ecdsa-sign", KeyType.ECDSA_P256)
        createdAliases.add(info.alias)

        val data = "test data to sign".toByteArray()
        val signature = keyManager.sign(info.alias, data)
        assertTrue(signature.isNotEmpty())

        // Verify signature with public key
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        val cert = ks.getCertificate(info.alias)
        val verifier = Signature.getInstance("SHA256withECDSA")
        verifier.initVerify(cert.publicKey)
        verifier.update(data)
        assertTrue(verifier.verify(signature))
    }

    @Test
    fun signEd25519_producesVerifiableSignature() {
        val info = keyManager.createKey("ed25519-sign", KeyType.ED25519)
        createdAliases.add(info.alias)

        val data = "test data to sign".toByteArray()
        val signature = keyManager.sign(info.alias, data)
        assertEquals(64, signature.size) // Ed25519 signatures are always 64 bytes

        // Verify signature using exported public key
        val sshKey = keyManager.exportPublicKey(info.alias)
        val blob = Base64.getDecoder().decode(sshKey.split(" ")[1])

        // Parse Ed25519 public key from SSH blob
        // Format: uint32 len + "ssh-ed25519" + uint32 len + 32-byte pubkey
        val rawPub = extractEd25519PubFromBlob(blob)
        val pubParams = Ed25519PublicKeyParameters(rawPub, 0)

        val verifier = Ed25519Signer()
        verifier.init(false, pubParams)
        verifier.update(data, 0, data.size)
        assertTrue(verifier.verifySignature(signature))
    }

    @Test
    fun deleteKey_removesEcdsaFromKeystore() {
        val info = keyManager.createKey("ecdsa-delete", KeyType.ECDSA_P256)

        keyManager.deleteKey(info.alias)

        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        assertFalse(ks.containsAlias(info.alias))

        val keys = keyManager.listKeys()
        assertFalse(keys.any { it.alias == info.alias })
    }

    @Test
    fun deleteKey_removesEd25519AndWrappingKey() {
        val info = keyManager.createKey("ed25519-delete", KeyType.ED25519)

        val wrappingAlias = info.wrappingKeyAlias!!
        keyManager.deleteKey(info.alias)

        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        assertFalse(ks.containsAlias(wrappingAlias))

        val keys = keyManager.listKeys()
        assertFalse(keys.any { it.alias == info.alias })
    }

    @Test
    fun ecdsaPrivateKey_isNotExtractable() {
        val info = keyManager.createKey("ecdsa-extract", KeyType.ECDSA_P256)
        createdAliases.add(info.alias)

        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        val entry = ks.getEntry(info.alias, null) as KeyStore.PrivateKeyEntry

        // Android Keystore private keys return null for getEncoded()
        // indicating the key material is not extractable
        val encoded = entry.privateKey.encoded
        assertTrue(
            "ECDSA private key should not be extractable",
            encoded == null || encoded.isEmpty(),
        )
    }

    @Test
    fun ed25519WrappingKey_isNotExtractable() {
        val info = keyManager.createKey("ed25519-extract", KeyType.ED25519)
        createdAliases.add(info.alias)

        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        val entry = ks.getEntry(info.wrappingKeyAlias, null) as KeyStore.SecretKeyEntry

        // AES wrapping key should not be extractable
        val encoded = entry.secretKey.encoded
        assertTrue(
            "AES wrapping key should not be extractable",
            encoded == null || encoded.isEmpty(),
        )
    }

    @Test
    fun signEcdsa_differentDataProducesDifferentSignatures() {
        val info = keyManager.createKey("ecdsa-diff", KeyType.ECDSA_P256)
        createdAliases.add(info.alias)

        val sig1 = keyManager.sign(info.alias, "data1".toByteArray())
        val sig2 = keyManager.sign(info.alias, "data2".toByteArray())

        assertFalse(sig1.contentEquals(sig2))
    }

    @Test
    fun signEd25519_differentDataProducesDifferentSignatures() {
        val info = keyManager.createKey("ed25519-diff", KeyType.ED25519)
        createdAliases.add(info.alias)

        val sig1 = keyManager.sign(info.alias, "data1".toByteArray())
        val sig2 = keyManager.sign(info.alias, "data2".toByteArray())

        assertFalse(sig1.contentEquals(sig2))
    }

    private fun extractEd25519PubFromBlob(blob: ByteArray): ByteArray {
        var offset = 0
        // Read key type string length
        val typeLen = readUint32(blob, offset)
        offset += 4 + typeLen
        // Read public key bytes length
        val pubLen = readUint32(blob, offset)
        offset += 4
        return blob.copyOfRange(offset, offset + pubLen)
    }

    private fun readUint32(data: ByteArray, offset: Int): Int {
        return ((data[offset].toInt() and 0xFF) shl 24) or
            ((data[offset + 1].toInt() and 0xFF) shl 16) or
            ((data[offset + 2].toInt() and 0xFF) shl 8) or
            (data[offset + 3].toInt() and 0xFF)
    }
}
