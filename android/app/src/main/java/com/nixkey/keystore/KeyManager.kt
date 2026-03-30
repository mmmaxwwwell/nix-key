package com.nixkey.keystore

import android.content.Context
import android.content.SharedPreferences
import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import dagger.hilt.android.qualifiers.ApplicationContext
import java.io.ByteArrayOutputStream
import java.io.DataOutputStream
import java.math.BigInteger
import java.nio.ByteBuffer
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.MessageDigest
import java.security.Signature
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import java.time.Instant
import java.util.Base64
import javax.annotation.concurrent.ThreadSafe
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.spec.GCMParameterSpec
import javax.inject.Inject
import javax.inject.Singleton
import org.bouncycastle.crypto.generators.Ed25519KeyPairGenerator
import org.bouncycastle.crypto.params.Ed25519KeyGenerationParameters
import org.bouncycastle.crypto.params.Ed25519PrivateKeyParameters
import org.bouncycastle.crypto.params.Ed25519PublicKeyParameters
import org.bouncycastle.crypto.signers.Ed25519Signer
import timber.log.Timber

@ThreadSafe
@Singleton
class KeyManager @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    private val keyStore: KeyStore = KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }
    private val prefs: SharedPreferences by lazy { createEncryptedPrefs() }

    fun createKey(
        name: String,
        type: KeyType,
        unlockPolicy: UnlockPolicy = UnlockPolicy.PASSWORD,
        signingPolicy: ConfirmationPolicy = ConfirmationPolicy.BIOMETRIC,
    ): SshKeyInfo {
        val alias = "nixkey_${type.name.lowercase()}_${System.nanoTime()}"

        return when (type) {
            KeyType.ECDSA_P256 -> createEcdsaKey(alias, name, unlockPolicy, signingPolicy)
            KeyType.ED25519 -> createEd25519Key(alias, name, unlockPolicy, signingPolicy)
        }
    }

    fun listKeys(): List<SshKeyInfo> {
        val aliases = prefs.getStringSet(PREF_KEY_ALIASES, emptySet()) ?: emptySet()
        return aliases.mapNotNull { alias -> loadKeyInfo(alias) }
    }

    fun exportPublicKey(alias: String): String {
        val info = loadKeyInfo(alias)
            ?: throw IllegalArgumentException("Key not found: $alias")

        val blob = when (info.keyType) {
            KeyType.ECDSA_P256 -> encodeEcdsaPublicKeyBlob(alias)
            KeyType.ED25519 -> encodeEd25519PublicKeyBlob(alias)
        }

        val encoded = Base64.getEncoder().encodeToString(blob)
        return "${info.keyType.sshName} $encoded ${info.displayName}"
    }

    fun sign(alias: String, data: ByteArray): ByteArray {
        val info = loadKeyInfo(alias)
            ?: throw IllegalArgumentException("Key not found: $alias")

        return when (info.keyType) {
            KeyType.ECDSA_P256 -> signEcdsa(alias, data)
            KeyType.ED25519 -> signEd25519(alias, data)
        }
    }

    fun updateKey(
        alias: String,
        displayName: String,
        unlockPolicy: UnlockPolicy,
        signingPolicy: ConfirmationPolicy,
    ) {
        val info = loadKeyInfo(alias)
            ?: throw IllegalArgumentException("Key not found: $alias")

        val updated = info.copy(
            displayName = displayName,
            unlockPolicy = unlockPolicy,
            confirmationPolicy = signingPolicy,
        )
        saveKeyInfo(updated)
        Timber.i(
            "Updated key alias=%s name=%s unlock=%s signing=%s",
            alias,
            displayName,
            unlockPolicy,
            signingPolicy,
        )
    }

    fun getKey(alias: String): SshKeyInfo? = loadKeyInfo(alias)

    fun deleteKey(alias: String) {
        val info = loadKeyInfo(alias)
            ?: throw IllegalArgumentException("Key not found: $alias")

        // Delete from Android Keystore
        if (info.keyType == KeyType.ECDSA_P256) {
            keyStore.deleteEntry(alias)
        }

        // Delete wrapping key for Ed25519
        if (info.wrappingKeyAlias != null) {
            keyStore.deleteEntry(info.wrappingKeyAlias)
        }

        // Remove metadata
        removeKeyInfo(alias)
        Timber.i("Deleted key alias=%s type=%s", alias, info.keyType)
    }

    // --- ECDSA-P256 (native Keystore) ---

    private fun createEcdsaKey(
        alias: String,
        name: String,
        unlockPolicy: UnlockPolicy,
        signingPolicy: ConfirmationPolicy,
    ): SshKeyInfo {
        val builder = KeyGenParameterSpec.Builder(
            alias,
            KeyProperties.PURPOSE_SIGN or KeyProperties.PURPOSE_VERIFY,
        )
            .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
            .setDigests(KeyProperties.DIGEST_SHA256)
            .setKeySize(256)

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            try {
                builder.setIsStrongBoxBacked(true)
                Timber.i("StrongBox available, using hardware backing for ECDSA key")
            } catch (_: Exception) {
                Timber.i("StrongBox not available, falling back to TEE")
            }
        }

        val kpg = KeyPairGenerator.getInstance(
            KeyProperties.KEY_ALGORITHM_EC,
            ANDROID_KEYSTORE,
        )
        try {
            kpg.initialize(builder.build())
            kpg.generateKeyPair()
        } catch (e: Exception) {
            // StrongBox may fail at generation time on some devices
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                val fallbackBuilder = KeyGenParameterSpec.Builder(
                    alias,
                    KeyProperties.PURPOSE_SIGN or KeyProperties.PURPOSE_VERIFY,
                )
                    .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_SHA256)
                    .setKeySize(256)
                kpg.initialize(fallbackBuilder.build())
                kpg.generateKeyPair()
                Timber.i("StrongBox generation failed, used TEE fallback")
            } else {
                throw e
            }
        }

        val fingerprint = computeFingerprint(encodeEcdsaPublicKeyBlob(alias))
        val info = SshKeyInfo(
            alias = alias,
            displayName = name,
            keyType = KeyType.ECDSA_P256,
            fingerprint = fingerprint,
            unlockPolicy = unlockPolicy,
            confirmationPolicy = signingPolicy,
            createdAt = Instant.now(),
            wrappingKeyAlias = null,
        )
        saveKeyInfo(info)
        Timber.i("Created ECDSA-P256 key alias=%s fingerprint=%s", alias, fingerprint)
        return info
    }

    private fun encodeEcdsaPublicKeyBlob(alias: String): ByteArray {
        val entry = keyStore.getCertificate(alias)
            ?: throw IllegalStateException("No certificate for alias: $alias")
        val ecPub = entry.publicKey as ECPublicKey
        return encodeSshEcdsaBlob(ecPub)
    }

    private fun signEcdsa(alias: String, data: ByteArray): ByteArray {
        val entry = keyStore.getEntry(alias, null) as KeyStore.PrivateKeyEntry
        val sig = Signature.getInstance("SHA256withECDSA")
        sig.initSign(entry.privateKey)
        sig.update(data)
        return sig.sign()
    }

    // --- Ed25519 (BouncyCastle + AES wrapping) ---

    private fun createEd25519Key(
        alias: String,
        name: String,
        unlockPolicy: UnlockPolicy,
        signingPolicy: ConfirmationPolicy,
    ): SshKeyInfo {
        val wrappingAlias = "${alias}_wrap"

        // Generate AES-256 wrapping key in Keystore
        val aesSpec = KeyGenParameterSpec.Builder(
            wrappingAlias,
            KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
        )
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setKeySize(256)
            .build()

        val aesGen = KeyGenerator.getInstance(
            KeyProperties.KEY_ALGORITHM_AES,
            ANDROID_KEYSTORE,
        )
        aesGen.init(aesSpec)
        aesGen.generateKey()

        // Generate Ed25519 key pair via BouncyCastle
        val kpGen = Ed25519KeyPairGenerator()
        kpGen.init(Ed25519KeyGenerationParameters(java.security.SecureRandom()))
        val keyPair = kpGen.generateKeyPair()

        val privateKey = keyPair.private as Ed25519PrivateKeyParameters
        val publicKey = keyPair.public as Ed25519PublicKeyParameters

        // Encrypt private key with AES wrapping key
        val wrappedPrivate = wrapEd25519PrivateKey(wrappingAlias, privateKey.encoded)

        // Store public key and wrapped private key in EncryptedSharedPreferences
        val pubEncoded = Base64.getEncoder().encodeToString(publicKey.encoded)
        val wrappedEncoded = Base64.getEncoder().encodeToString(wrappedPrivate)

        prefs.edit()
            .putString("${alias}_ed25519_pub", pubEncoded)
            .putString("${alias}_ed25519_priv", wrappedEncoded)
            .apply()

        val fingerprint = computeFingerprint(encodeEd25519PublicKeyBlob(alias))
        val info = SshKeyInfo(
            alias = alias,
            displayName = name,
            keyType = KeyType.ED25519,
            fingerprint = fingerprint,
            unlockPolicy = unlockPolicy,
            confirmationPolicy = signingPolicy,
            createdAt = Instant.now(),
            wrappingKeyAlias = wrappingAlias,
        )
        saveKeyInfo(info)
        Timber.i("Created Ed25519 key alias=%s fingerprint=%s", alias, fingerprint)
        return info
    }

    private fun wrapEd25519PrivateKey(wrappingAlias: String, privateKeyBytes: ByteArray): ByteArray {
        val secretKey = (keyStore.getEntry(wrappingAlias, null) as KeyStore.SecretKeyEntry).secretKey
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, secretKey)
        val iv = cipher.iv
        val encrypted = cipher.doFinal(privateKeyBytes)
        // Prefix IV length + IV + encrypted data
        val buf = ByteBuffer.allocate(4 + iv.size + encrypted.size)
        buf.putInt(iv.size)
        buf.put(iv)
        buf.put(encrypted)
        return buf.array()
    }

    private fun unwrapEd25519PrivateKey(wrappingAlias: String, wrapped: ByteArray): ByteArray {
        val buf = ByteBuffer.wrap(wrapped)
        val ivLen = buf.getInt()
        val iv = ByteArray(ivLen)
        buf.get(iv)
        val encrypted = ByteArray(buf.remaining())
        buf.get(encrypted)

        val secretKey = (keyStore.getEntry(wrappingAlias, null) as KeyStore.SecretKeyEntry).secretKey
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, secretKey, GCMParameterSpec(GCM_TAG_LENGTH, iv))
        return cipher.doFinal(encrypted)
    }

    private fun encodeEd25519PublicKeyBlob(alias: String): ByteArray {
        val pubEncoded = prefs.getString("${alias}_ed25519_pub", null)
            ?: throw IllegalStateException("No Ed25519 public key for alias: $alias")
        val rawPub = Base64.getDecoder().decode(pubEncoded)
        return encodeSshEd25519Blob(rawPub)
    }

    private fun signEd25519(alias: String, data: ByteArray): ByteArray {
        val info = loadKeyInfo(alias)
            ?: throw IllegalStateException("Key metadata not found: $alias")
        val wrappingAlias = info.wrappingKeyAlias
            ?: throw IllegalStateException("No wrapping key for Ed25519 alias: $alias")

        val wrappedEncoded = prefs.getString("${alias}_ed25519_priv", null)
            ?: throw IllegalStateException("No Ed25519 private key for alias: $alias")
        val wrapped = Base64.getDecoder().decode(wrappedEncoded)

        val privateKeyBytes = unwrapEd25519PrivateKey(wrappingAlias, wrapped)
        try {
            val privateKey = Ed25519PrivateKeyParameters(privateKeyBytes, 0)
            val signer = Ed25519Signer()
            signer.init(true, privateKey)
            signer.update(data, 0, data.size)
            return signer.generateSignature()
        } finally {
            // Clear private key bytes from memory
            privateKeyBytes.fill(0)
        }
    }

    // --- SSH encoding helpers ---

    private fun encodeSshEcdsaBlob(pubKey: ECPublicKey): ByteArray {
        val out = ByteArrayOutputStream()
        val dos = DataOutputStream(out)
        val keyType = "ecdsa-sha2-nistp256"
        val curveName = "nistp256"

        writeSshString(dos, keyType)
        writeSshString(dos, curveName)

        // EC point in uncompressed form: 0x04 || x || y
        val x = pubKey.w.affineX
        val y = pubKey.w.affineY
        val point = encodeEcPoint(x, y)
        writeSshBytes(dos, point)

        return out.toByteArray()
    }

    private fun encodeSshEd25519Blob(rawPublicKey: ByteArray): ByteArray {
        val out = ByteArrayOutputStream()
        val dos = DataOutputStream(out)
        writeSshString(dos, "ssh-ed25519")
        writeSshBytes(dos, rawPublicKey)
        return out.toByteArray()
    }

    private fun writeSshString(dos: DataOutputStream, s: String) {
        val bytes = s.toByteArray(Charsets.UTF_8)
        dos.writeInt(bytes.size)
        dos.write(bytes)
    }

    private fun writeSshBytes(dos: DataOutputStream, bytes: ByteArray) {
        dos.writeInt(bytes.size)
        dos.write(bytes)
    }

    private fun encodeEcPoint(x: BigInteger, y: BigInteger): ByteArray {
        val xBytes = toUnsignedFixedLength(x, 32)
        val yBytes = toUnsignedFixedLength(y, 32)
        val point = ByteArray(1 + 32 + 32)
        point[0] = 0x04 // uncompressed
        System.arraycopy(xBytes, 0, point, 1, 32)
        System.arraycopy(yBytes, 0, point, 33, 32)
        return point
    }

    private fun toUnsignedFixedLength(value: BigInteger, length: Int): ByteArray {
        val bytes = value.toByteArray()
        return when {
            bytes.size == length -> bytes
            bytes.size > length -> bytes.copyOfRange(bytes.size - length, bytes.size)
            else -> ByteArray(length - bytes.size) + bytes
        }
    }

    private fun computeFingerprint(blob: ByteArray): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(blob)
        return "SHA256:" + Base64.getEncoder().encodeToString(digest).trimEnd('=')
    }

    // --- Metadata persistence ---

    private fun saveKeyInfo(info: SshKeyInfo) {
        val aliases = prefs.getStringSet(PREF_KEY_ALIASES, mutableSetOf())?.toMutableSet()
            ?: mutableSetOf()
        aliases.add(info.alias)

        prefs.edit()
            .putStringSet(PREF_KEY_ALIASES, aliases)
            .putString("${info.alias}_name", info.displayName)
            .putString("${info.alias}_type", info.keyType.name)
            .putString("${info.alias}_fingerprint", info.fingerprint)
            .putString("${info.alias}_unlock_policy", info.unlockPolicy.name)
            .putString("${info.alias}_policy", info.confirmationPolicy.name)
            .putString("${info.alias}_created", info.createdAt.toString())
            .putString("${info.alias}_wrapping", info.wrappingKeyAlias)
            .apply()
    }

    private fun loadKeyInfo(alias: String): SshKeyInfo? {
        val typeName = prefs.getString("${alias}_type", null) ?: return null
        return try {
            SshKeyInfo(
                alias = alias,
                displayName = prefs.getString("${alias}_name", "") ?: "",
                keyType = KeyType.valueOf(typeName),
                fingerprint = prefs.getString("${alias}_fingerprint", "") ?: "",
                unlockPolicy = try {
                    UnlockPolicy.valueOf(
                        prefs.getString("${alias}_unlock_policy", "PASSWORD") ?: "PASSWORD",
                    )
                } catch (_: IllegalArgumentException) {
                    UnlockPolicy.PASSWORD
                },
                confirmationPolicy = ConfirmationPolicy.valueOf(
                    prefs.getString("${alias}_policy", "BIOMETRIC") ?: "BIOMETRIC",
                ),
                createdAt = Instant.parse(
                    prefs.getString("${alias}_created", Instant.EPOCH.toString()),
                ),
                wrappingKeyAlias = prefs.getString("${alias}_wrapping", null),
            )
        } catch (e: Exception) {
            Timber.e(e, "Failed to load key info for alias=%s", alias)
            null
        }
    }

    private fun removeKeyInfo(alias: String) {
        val aliases = prefs.getStringSet(PREF_KEY_ALIASES, mutableSetOf())?.toMutableSet()
            ?: mutableSetOf()
        aliases.remove(alias)

        prefs.edit()
            .putStringSet(PREF_KEY_ALIASES, aliases)
            .remove("${alias}_name")
            .remove("${alias}_type")
            .remove("${alias}_fingerprint")
            .remove("${alias}_unlock_policy")
            .remove("${alias}_policy")
            .remove("${alias}_created")
            .remove("${alias}_wrapping")
            .remove("${alias}_ed25519_pub")
            .remove("${alias}_ed25519_priv")
            .apply()
    }

    private fun createEncryptedPrefs(): SharedPreferences {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()

        return EncryptedSharedPreferences.create(
            context,
            PREFS_FILE,
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    }

    companion object {
        private const val ANDROID_KEYSTORE = "AndroidKeyStore"
        private const val PREFS_FILE = "nixkey_keys"
        private const val PREF_KEY_ALIASES = "key_aliases"
        private const val GCM_TAG_LENGTH = 128
    }
}
