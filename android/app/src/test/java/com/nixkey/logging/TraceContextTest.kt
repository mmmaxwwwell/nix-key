package com.nixkey.logging

import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class TraceContextTest {
    @After
    fun tearDown() {
        TraceContext.clear()
    }

    @Test
    fun `currentTraceId returns null when not set`() {
        assertNull(TraceContext.currentTraceId())
    }

    @Test
    fun `setTraceId makes id available via currentTraceId`() {
        TraceContext.setTraceId("abc123")
        assertEquals("abc123", TraceContext.currentTraceId())
    }

    @Test
    fun `clear removes trace id`() {
        TraceContext.setTraceId("abc123")
        TraceContext.clear()
        assertNull(TraceContext.currentTraceId())
    }

    @Test
    fun `withTraceId sets and clears trace id around block`() {
        val result = TraceContext.withTraceId("trace-456") {
            assertEquals("trace-456", TraceContext.currentTraceId())
            "done"
        }
        assertEquals("done", result)
        assertNull(TraceContext.currentTraceId())
    }

    @Test
    fun `withTraceId clears on exception`() {
        try {
            TraceContext.withTraceId("trace-err") {
                throw RuntimeException("test")
            }
        } catch (_: RuntimeException) {
            // expected
        }
        assertNull(TraceContext.currentTraceId())
    }
}
