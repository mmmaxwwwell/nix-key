package com.nixkey.bridge

import com.nixkey.keystore.ConfirmationPolicy
import com.nixkey.keystore.KeyManager
import com.nixkey.keystore.KeyType
import com.nixkey.keystore.KeyUnlockManager
import com.nixkey.keystore.SignRequest
import com.nixkey.keystore.SignRequestQueue
import com.nixkey.keystore.SignRequestStatus
import com.nixkey.keystore.UnlockPolicy
import phoneserver.Confirmer
import phoneserver.Key
import phoneserver.KeyList
import phoneserver.KeyStore
import phoneserver.PhoneServer
import phoneserver.Phoneserver
import timber.log.Timber
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicReference
import javax.annotation.concurrent.GuardedBy
import javax.annotation.concurrent.ThreadSafe
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Kotlin adapter that implements the gomobile-generated [KeyStore] interface
 * by delegating to [KeyManager].
 *
 * gomobile generates Java interfaces from Go interfaces in pkg/phoneserver.
 * This class bridges the Go world (KeyStore, Key, KeyList) to the Android
 * world (KeyManager, SshKeyInfo).
 */
class KeyStoreAdapter(
    private val keyManager: KeyManager,
) : KeyStore {

    override fun listKeys(): KeyList {
        val keys = keyManager.listKeys()
        val keyList = Phoneserver.newKeyList()
        for (info in keys) {
            val key = Key()
            key.publicKeyBlob = getPublicKeyBlob(info.alias, info.keyType)
            key.keyType = info.keyType.sshName
            key.displayName = info.displayName
            key.fingerprint = info.fingerprint
            keyList.add(key)
        }
        return keyList
    }

    override fun sign(fingerprint: String?, data: ByteArray?, flags: Int): ByteArray {
        val fp = fingerprint ?: throw IllegalArgumentException("fingerprint is null")
        val d = data ?: throw IllegalArgumentException("data is null")
        val keys = keyManager.listKeys()
        val info = keys.find { it.fingerprint == fp }
            ?: throw IllegalArgumentException("Key not found: $fp")
        return keyManager.sign(info.alias, d)
    }

    private fun getPublicKeyBlob(alias: String, keyType: KeyType): ByteArray {
        // exportPublicKey returns "type base64 comment", we need the raw blob
        val pubKeyString = keyManager.exportPublicKey(alias)
        val parts = pubKeyString.split(" ")
        if (parts.size < 2) {
            throw IllegalStateException("Invalid SSH public key format for alias: $alias")
        }
        return java.util.Base64.getDecoder().decode(parts[1])
    }
}

/**
 * Kotlin adapter that implements the gomobile-generated [Confirmer] interface
 * by delegating to [SignRequestQueue] for UI display.
 *
 * The Go gRPC server calls [requestConfirmation] from a background thread.
 * This adapter enqueues a [SignRequest] into the queue (observed by the
 * Compose UI) and blocks until the user responds or a timeout occurs.
 *
 * If the key is locked, the request is enqueued with [SignRequest.needsUnlock]
 * set to true. The UI layer handles the unlock prompt first, then proceeds
 * to the signing confirmation.
 */
@ThreadSafe
class ConfirmerAdapter(
    private val signRequestQueue: SignRequestQueue,
    private val keyManager: KeyManager,
    private val keyUnlockManager: KeyUnlockManager,
    private val confirmationTimeoutSeconds: Long = 60,
) : Confirmer {

    override fun requestConfirmation(
        hostName: String?,
        keyName: String?,
        dataHash: String?,
    ): Boolean {
        val host = hostName ?: "unknown"
        val key = keyName ?: "unknown"
        val hash = dataHash ?: ""
        val latch = CountDownLatch(1)
        val approved = AtomicBoolean(false)
        val requestIdRef = AtomicReference<String>()

        // Look up the key's policies from KeyManager
        val keyInfo = keyManager.listKeys().find {
            it.displayName == key || it.fingerprint == key
        }
        val signingPolicy = keyInfo?.confirmationPolicy ?: ConfirmationPolicy.BIOMETRIC
        val unlockPolicy = keyInfo?.unlockPolicy ?: UnlockPolicy.PASSWORD
        val fingerprint = keyInfo?.fingerprint ?: ""

        // Check if key needs unlock
        val needsUnlock = keyInfo != null && !keyUnlockManager.isUnlocked(fingerprint)

        val request = SignRequest(
            keyFingerprint = fingerprint,
            hostName = host,
            keyName = key,
            dataToSign = hash.toByteArray(Charsets.UTF_8),
            unlockPolicy = unlockPolicy,
            confirmationPolicy = signingPolicy,
            needsUnlock = needsUnlock,
        )
        requestIdRef.set(request.requestId)

        // Register a callback to unblock when the request is completed
        val observer = object : ConfirmationObserver {
            override fun onCompleted(requestId: String, status: SignRequestStatus) {
                if (requestId == requestIdRef.get()) {
                    approved.set(status == SignRequestStatus.APPROVED)
                    latch.countDown()
                }
            }
        }
        addObserver(observer)

        Timber.i(
            "Confirmer: enqueuing request id=%s host=%s key=%s needsUnlock=%s",
            request.requestId,
            host,
            key,
            needsUnlock,
        )
        signRequestQueue.enqueue(request)

        // Block until user responds or timeout
        val responded = latch.await(confirmationTimeoutSeconds, TimeUnit.SECONDS)
        removeObserver(observer)

        if (!responded) {
            Timber.w("Confirmer: timeout waiting for user response, request=%s", request.requestId)
            signRequestQueue.complete(request.requestId, SignRequestStatus.TIMEOUT)
            return false
        }

        return approved.get()
    }

    interface ConfirmationObserver {
        fun onCompleted(requestId: String, status: SignRequestStatus)
    }

    @GuardedBy("observerLock")
    private val observers = mutableListOf<ConfirmationObserver>()
    private val observerLock = Any()

    fun addObserver(observer: ConfirmationObserver) {
        synchronized(observerLock) {
            observers.add(observer)
        }
    }

    fun removeObserver(observer: ConfirmationObserver) {
        synchronized(observerLock) {
            observers.remove(observer)
        }
    }

    /**
     * Called by the UI layer when a sign request is approved or denied.
     * Notifies the blocking [requestConfirmation] call.
     */
    fun notifyCompletion(requestId: String, status: SignRequestStatus) {
        synchronized(observerLock) {
            observers.toList().forEach { it.onCompleted(requestId, status) }
        }
    }
}

/**
 * Main bridge between the Go gRPC phone server and Android components.
 *
 * Wraps the gomobile-generated [PhoneServer] and manages its lifecycle.
 * The Go server runs in a background thread and delegates key operations
 * to [KeyManager] via [KeyStoreAdapter] and user confirmation to the
 * Compose UI via [ConfirmerAdapter].
 */
@ThreadSafe
@Singleton
class GoPhoneServer @Inject constructor(
    private val keyManager: KeyManager,
    private val signRequestQueue: SignRequestQueue,
    private val keyUnlockManager: KeyUnlockManager,
) {
    @Volatile
    private var phoneServer: PhoneServer? = null

    @Volatile
    private var serverThread: Thread? = null
    private val running = AtomicBoolean(false)

    val keyStoreAdapter: KeyStoreAdapter by lazy { KeyStoreAdapter(keyManager) }
    val confirmerAdapter: ConfirmerAdapter by lazy {
        ConfirmerAdapter(signRequestQueue, keyManager, keyUnlockManager)
    }

    /**
     * Start the gRPC server on the given address (e.g., "0.0.0.0:50051").
     * Returns immediately; the server runs in a background thread.
     *
     * @param address The address to listen on
     * @param otelEndpoint Optional OTEL collector endpoint for trace export
     * @throws IllegalStateException if the server is already running
     */
    fun start(address: String, otelEndpoint: String? = null) {
        if (running.getAndSet(true)) {
            throw IllegalStateException("GoPhoneServer is already running")
        }

        val ks = keyStoreAdapter
        val conf = confirmerAdapter
        val ps = Phoneserver.newPhoneServer(ks, conf)
        if (!otelEndpoint.isNullOrEmpty()) {
            ps.setOTELEndpoint(otelEndpoint)
            Timber.i("GoPhoneServer OTEL endpoint set to %s", otelEndpoint)
        }
        phoneServer = ps

        serverThread = Thread({
            try {
                Timber.i("GoPhoneServer starting on %s", address)
                ps.startOnAddress(address)
            } catch (e: Exception) {
                if (running.get()) {
                    Timber.e(e, "GoPhoneServer error")
                }
            } finally {
                running.set(false)
                Timber.i("GoPhoneServer stopped")
            }
        }, "go-phone-server")
        serverThread?.isDaemon = true
        serverThread?.start()
    }

    /**
     * Returns the port the server is listening on, or 0 if not started.
     */
    // gomobile maps Go `int` to Java `long`; convert to Int for convenience
    fun port(): Int = phoneServer?.port()?.toInt() ?: 0

    /**
     * Returns true if the server is currently running.
     */
    fun isRunning(): Boolean = running.get()

    /**
     * Gracefully stop the gRPC server.
     */
    fun stop() {
        if (!running.getAndSet(false)) {
            return
        }
        Timber.i("GoPhoneServer stopping")
        phoneServer?.stop()
        serverThread?.join(5000)
        phoneServer = null
        serverThread = null
    }
}
