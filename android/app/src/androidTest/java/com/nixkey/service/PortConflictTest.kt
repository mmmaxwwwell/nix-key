package com.nixkey.service

import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.AndroidJUnit4
import com.nixkey.bridge.GoPhoneServer
import com.nixkey.keystore.KeyManager
import com.nixkey.keystore.KeyUnlockManager
import com.nixkey.keystore.SignRequestQueue
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import java.net.ServerSocket

/**
 * T-AI-17: gRPC port conflict error (FR-E19).
 *
 * Verifies that GoPhoneServer properly reports errors when the requested port
 * is already in use, and that GrpcServerService's port conflict detection
 * logic handles BindException and "address already in use" messages.
 */
@RunWith(AndroidJUnit4::class)
class PortConflictTest {

    private lateinit var keyManager: KeyManager
    private lateinit var signRequestQueue: SignRequestQueue
    private lateinit var keyUnlockManager: KeyUnlockManager
    private lateinit var goPhoneServer: GoPhoneServer

    @Before
    fun setUp() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        keyManager = KeyManager(context)
        signRequestQueue = SignRequestQueue()
        keyUnlockManager = KeyUnlockManager()
        goPhoneServer = GoPhoneServer(keyManager, signRequestQueue, keyUnlockManager)
    }

    @After
    fun tearDown() {
        goPhoneServer.stop()
    }

    @Test
    fun doubleStart_throwsIllegalStateException() {
        goPhoneServer.start("127.0.0.1:0")
        Thread.sleep(500)
        assertTrue("Server should be running", goPhoneServer.isRunning())

        try {
            goPhoneServer.start("127.0.0.1:0")
            fail("Second start should throw IllegalStateException")
        } catch (e: IllegalStateException) {
            assertTrue(
                "Error should indicate already running",
                e.message!!.contains("already running"),
            )
        }
    }

    @Test
    fun startOnOccupiedPort_serverStopsGracefully() {
        // Bind a port with a Java ServerSocket first
        val blocker = ServerSocket(0)
        val blockedPort = blocker.localPort
        assertTrue("Blocker should be bound", blockedPort > 0)

        try {
            // Attempt to start GoPhoneServer on the occupied port.
            // The Go server runs in a background thread and the start call
            // may not immediately fail (the Go layer tries to bind asynchronously).
            // We verify that either: (a) it throws immediately, or (b) it stops
            // running shortly after due to the bind failure.
            try {
                goPhoneServer.start("127.0.0.1:$blockedPort")
                // Give the background thread time to encounter the bind error
                Thread.sleep(2000)
                // The server should have stopped due to bind failure
                assertFalse(
                    "Server should not remain running on occupied port",
                    goPhoneServer.isRunning(),
                )
            } catch (e: Exception) {
                // Immediate failure is also acceptable
                assertTrue(
                    "Exception should relate to port binding",
                    e.message?.contains("address already in use", ignoreCase = true) == true ||
                        e.message?.contains("EADDRINUSE", ignoreCase = true) == true ||
                        e.message?.contains("bind", ignoreCase = true) == true ||
                        e is java.net.BindException,
                )
            }
        } finally {
            blocker.close()
        }
    }

    @Test
    fun portConflictDetection_bindExceptionMessage() {
        // Test the GrpcServerService port conflict detection logic directly
        val bindException = java.net.BindException("Address already in use")
        assertTrue(
            "BindException should be detected as port conflict",
            isPortConflict(bindException),
        )
    }

    @Test
    fun portConflictDetection_wrappedException() {
        // When gomobile wraps exceptions, BindException may be the cause
        val wrapped = Exception(
            "server start failed",
            java.net.BindException("Address already in use"),
        )
        assertTrue(
            "Wrapped BindException should be detected",
            isPortConflict(wrapped),
        )
    }

    @Test
    fun portConflictDetection_eaddrinuseMessage() {
        // Go-level errors may use EADDRINUSE in the message
        val goError = Exception("listen tcp 100.64.0.1:29418: bind: EADDRINUSE")
        assertTrue(
            "EADDRINUSE message should be detected",
            isPortConflict(goError),
        )
    }

    @Test
    fun portConflictDetection_unrelatedError() {
        val unrelated = Exception("network unreachable")
        assertFalse(
            "Unrelated error should not be detected as port conflict",
            isPortConflict(unrelated),
        )
    }

    @Test
    fun startStopStart_works() {
        goPhoneServer.start("127.0.0.1:0")
        Thread.sleep(500)
        assertTrue(goPhoneServer.isRunning())
        val firstPort = goPhoneServer.port()
        assertTrue("First port should be assigned", firstPort > 0)

        goPhoneServer.stop()
        assertFalse("Server should be stopped", goPhoneServer.isRunning())

        // Restart on fresh port
        goPhoneServer.start("127.0.0.1:0")
        Thread.sleep(500)
        assertTrue("Server should be running again", goPhoneServer.isRunning())
        assertTrue("New port should be assigned", goPhoneServer.port() > 0)
    }

    /**
     * Mirrors the port conflict detection logic from [GrpcServerService.startServer].
     */
    private fun isPortConflict(e: Exception): Boolean {
        if (e is java.net.BindException) return true
        return e.cause is java.net.BindException ||
            e.message?.contains("address already in use", ignoreCase = true) == true ||
            e.message?.contains("EADDRINUSE", ignoreCase = true) == true
    }
}
