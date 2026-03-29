package com.nixkey.keystore

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import timber.log.Timber
import java.util.concurrent.ConcurrentLinkedQueue
import javax.inject.Inject
import javax.inject.Singleton

/**
 * FIFO queue for concurrent sign requests. Shows one request at a time.
 *
 * When a sign request arrives, it is enqueued. The current (front-of-queue) request
 * is exposed via [currentRequest]. When the user approves or denies, the request is
 * completed and the next one (if any) is surfaced.
 *
 * Thread-safe: uses ConcurrentLinkedQueue for the backing store and StateFlow for
 * reactive UI updates.
 */
@Singleton
class SignRequestQueue @Inject constructor() {

    private val queue = ConcurrentLinkedQueue<SignRequest>()
    private val _currentRequest = MutableStateFlow<SignRequest?>(null)

    /** The request currently being shown to the user, or null if the queue is empty. */
    val currentRequest: StateFlow<SignRequest?> = _currentRequest.asStateFlow()

    private val _queueSize = MutableStateFlow(0)

    /** Number of pending requests in the queue (including the one currently displayed). */
    val queueSize: StateFlow<Int> = _queueSize.asStateFlow()

    /**
     * Enqueue a new sign request. If no request is currently displayed, it becomes
     * the current request immediately.
     */
    fun enqueue(request: SignRequest) {
        Timber.i(
            "Sign request enqueued: id=%s host=%s key=%s",
            request.requestId,
            request.hostName,
            request.keyName,
        )
        queue.add(request)
        _queueSize.value = queue.size
        if (_currentRequest.value == null) {
            advanceQueue()
        }
    }

    /**
     * Complete the current request (approved or denied) and advance to the next one.
     *
     * @param requestId The request ID being completed (must match current)
     * @param status The final status (APPROVED or DENIED)
     * @return The completed request with updated status, or null if no match
     */
    fun complete(requestId: String, status: SignRequestStatus): SignRequest? {
        val current = _currentRequest.value
        if (current == null || current.requestId != requestId) {
            Timber.w("Attempted to complete non-current request: %s", requestId)
            return null
        }
        val completed = current.copy(status = status)
        Timber.i(
            "Sign request completed: id=%s status=%s",
            requestId,
            status,
        )
        advanceQueue()
        return completed
    }

    /**
     * Remove all pending requests from the queue. Useful on disconnect or timeout.
     */
    fun clear() {
        queue.clear()
        _currentRequest.value = null
        _queueSize.value = 0
        Timber.i("Sign request queue cleared")
    }

    private fun advanceQueue() {
        val next = queue.poll()
        _currentRequest.value = next
        _queueSize.value = queue.size
    }
}
