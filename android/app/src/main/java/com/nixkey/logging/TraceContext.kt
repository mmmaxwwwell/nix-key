package com.nixkey.logging

/**
 * Thread-local trace context for correlating log entries with OTEL trace IDs.
 * When OTEL is enabled, the trace ID is set before processing a request
 * and cleared afterward, ensuring all log entries within the request
 * scope include the trace ID.
 */
object TraceContext {
    private val traceId = ThreadLocal<String?>()

    fun setTraceId(id: String?) {
        traceId.set(id)
    }

    fun currentTraceId(): String? = traceId.get()

    fun clear() {
        traceId.remove()
    }

    /**
     * Execute a block with a trace ID set in the current thread's context.
     * The trace ID is automatically cleared after the block completes.
     */
    inline fun <T> withTraceId(id: String, block: () -> T): T {
        setTraceId(id)
        try {
            return block()
        } finally {
            clear()
        }
    }
}
