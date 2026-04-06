package com.nixkey.logging

import android.util.Log
import java.time.Instant
import java.time.format.DateTimeFormatter
import org.json.JSONObject
import timber.log.Timber

/**
 * Timber tree that outputs structured JSON log lines to logcat.
 * Mirrors the host-side structured logger format for consistency.
 *
 * Output format:
 * {"timestamp":"2026-03-29T12:00:00Z","level":"INFO","module":"logging","message":"...","traceId":"..."}
 */
class JsonTree : Timber.Tree() {
    override fun log(priority: Int, tag: String?, message: String, t: Throwable?) {
        val json = JSONObject().apply {
            put("timestamp", DateTimeFormatter.ISO_INSTANT.format(Instant.now()))
            put("level", priorityToLevel(priority))
            put("module", tag ?: "app")
            put("message", message)

            // Extract trace ID from thread-local if OTEL is enabled
            TraceContext.currentTraceId()?.let { put("traceId", it) }

            t?.let { put("error", it.toString()) }
        }

        Log.println(priority, "nix-key", json.toString())
    }

    private fun priorityToLevel(priority: Int): String = when (priority) {
        Log.VERBOSE -> "TRACE"
        Log.DEBUG -> "DEBUG"
        Log.INFO -> "INFO"
        Log.WARN -> "WARN"
        Log.ERROR -> "ERROR"
        Log.ASSERT -> "FATAL"
        else -> "UNKNOWN"
    }
}
