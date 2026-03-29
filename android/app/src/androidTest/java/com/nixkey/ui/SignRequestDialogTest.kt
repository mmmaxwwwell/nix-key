package com.nixkey.ui

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.nixkey.keystore.ConfirmationPolicy
import com.nixkey.keystore.SignRequest
import com.nixkey.keystore.SignRequestQueue
import com.nixkey.keystore.SignRequestStatus
import com.nixkey.ui.screens.SignRequestDialog
import com.nixkey.ui.screens.SignRequestDialogContent
import com.nixkey.ui.theme.NixKeyTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class SignRequestDialogTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun makeRequest(
        hostName: String = "dev-workstation",
        keyName: String = "my-ed25519-key",
        data: ByteArray = "test data to sign".toByteArray(),
        policy: ConfirmationPolicy = ConfirmationPolicy.ALWAYS_ASK,
    ) = SignRequest(
        keyFingerprint = "SHA256:abc123",
        hostName = hostName,
        keyName = keyName,
        dataToSign = data,
        confirmationPolicy = policy,
    )

    @Test
    fun dialogContent_showsRequestDetails() {
        val request = makeRequest()

        composeTestRule.setContent {
            NixKeyTheme {
                SignRequestDialogContent(
                    request = request,
                    onApprove = {},
                    onDeny = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Sign Request").assertIsDisplayed()
        composeTestRule.onNodeWithText("dev-workstation").assertIsDisplayed()
        composeTestRule.onNodeWithText("my-ed25519-key").assertIsDisplayed()
        composeTestRule.onNodeWithText("Approve").assertIsDisplayed()
        composeTestRule.onNodeWithText("Deny").assertIsDisplayed()
    }

    @Test
    fun dialogContent_showsDataHash() {
        val request = makeRequest()
        val expectedHash = request.dataHashTruncated()

        composeTestRule.setContent {
            NixKeyTheme {
                SignRequestDialogContent(
                    request = request,
                    onApprove = {},
                    onDeny = {},
                )
            }
        }

        composeTestRule.onNodeWithText(expectedHash).assertIsDisplayed()
    }

    @Test
    fun dialogContent_showsQueueCount() {
        val request = makeRequest()

        composeTestRule.setContent {
            NixKeyTheme {
                SignRequestDialogContent(
                    request = request,
                    queueSize = 3,
                    onApprove = {},
                    onDeny = {},
                )
            }
        }

        composeTestRule.onNodeWithText("3 more requests waiting").assertIsDisplayed()
    }

    @Test
    fun dialogContent_showsSingularQueueCount() {
        val request = makeRequest()

        composeTestRule.setContent {
            NixKeyTheme {
                SignRequestDialogContent(
                    request = request,
                    queueSize = 1,
                    onApprove = {},
                    onDeny = {},
                )
            }
        }

        composeTestRule.onNodeWithText("1 more request waiting").assertIsDisplayed()
    }

    @Test
    fun dialogContent_hidesQueueCountWhenZero() {
        val request = makeRequest()

        composeTestRule.setContent {
            NixKeyTheme {
                SignRequestDialogContent(
                    request = request,
                    queueSize = 0,
                    onApprove = {},
                    onDeny = {},
                )
            }
        }

        composeTestRule.onNodeWithTag("queue_count").assertDoesNotExist()
    }

    @Test
    fun dialogContent_approveButtonCallsCallback() {
        var approved = false
        val request = makeRequest()

        composeTestRule.setContent {
            NixKeyTheme {
                SignRequestDialogContent(
                    request = request,
                    onApprove = { approved = true },
                    onDeny = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Approve").performClick()
        assertTrue(approved)
    }

    @Test
    fun dialogContent_denyButtonCallsCallback() {
        var denied = false
        val request = makeRequest()

        composeTestRule.setContent {
            NixKeyTheme {
                SignRequestDialogContent(
                    request = request,
                    onApprove = {},
                    onDeny = { denied = true },
                )
            }
        }

        composeTestRule.onNodeWithText("Deny").performClick()
        assertTrue(denied)
    }

    @Test
    fun dialog_notShownWhenQueueEmpty() {
        val queue = SignRequestQueue()

        composeTestRule.setContent {
            NixKeyTheme {
                SignRequestDialog(
                    queue = queue,
                    onApprove = {},
                    onDeny = {},
                )
            }
        }

        composeTestRule.onNodeWithTag("sign_request_dialog").assertDoesNotExist()
    }

    @Test
    fun dialog_shownWhenRequestEnqueued() {
        val queue = SignRequestQueue()
        val request = makeRequest()

        composeTestRule.setContent {
            NixKeyTheme {
                SignRequestDialog(
                    queue = queue,
                    onApprove = {},
                    onDeny = {},
                )
            }
        }

        queue.enqueue(request)
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithTag("sign_request_dialog").assertIsDisplayed()
        composeTestRule.onNodeWithText("dev-workstation").assertIsDisplayed()
    }

    @Test
    fun queue_fifoOrder() {
        val queue = SignRequestQueue()
        val request1 = makeRequest(hostName = "host-1")
        val request2 = makeRequest(hostName = "host-2")
        val request3 = makeRequest(hostName = "host-3")

        // Enqueue three requests
        queue.enqueue(request1)
        queue.enqueue(request2)
        queue.enqueue(request3)

        // First should be current
        assertEquals("host-1", queue.currentRequest.value?.hostName)
        assertEquals(2, queue.queueSize.value)

        // Complete first -> second becomes current
        queue.complete(request1.requestId, SignRequestStatus.APPROVED)
        assertEquals("host-2", queue.currentRequest.value?.hostName)
        assertEquals(1, queue.queueSize.value)

        // Complete second -> third becomes current
        queue.complete(request2.requestId, SignRequestStatus.DENIED)
        assertEquals("host-3", queue.currentRequest.value?.hostName)
        assertEquals(0, queue.queueSize.value)

        // Complete third -> queue is empty
        queue.complete(request3.requestId, SignRequestStatus.APPROVED)
        assertNull(queue.currentRequest.value)
        assertEquals(0, queue.queueSize.value)
    }

    @Test
    fun queue_completeReturnsUpdatedRequest() {
        val queue = SignRequestQueue()
        val request = makeRequest()

        queue.enqueue(request)
        val completed = queue.complete(request.requestId, SignRequestStatus.APPROVED)

        assertEquals(SignRequestStatus.APPROVED, completed?.status)
        assertEquals(request.requestId, completed?.requestId)
    }

    @Test
    fun queue_completeWrongIdReturnsNull() {
        val queue = SignRequestQueue()
        val request = makeRequest()

        queue.enqueue(request)
        val result = queue.complete("wrong-id", SignRequestStatus.APPROVED)

        assertNull(result)
        // Original request should still be current
        assertEquals(request.requestId, queue.currentRequest.value?.requestId)
    }

    @Test
    fun queue_clearRemovesAllRequests() {
        val queue = SignRequestQueue()
        queue.enqueue(makeRequest(hostName = "host-1"))
        queue.enqueue(makeRequest(hostName = "host-2"))

        queue.clear()

        assertNull(queue.currentRequest.value)
        assertEquals(0, queue.queueSize.value)
    }

    @Test
    fun dialog_advancesAfterApprove() {
        val queue = SignRequestQueue()
        val request1 = makeRequest(hostName = "host-1")
        val request2 = makeRequest(hostName = "host-2")

        composeTestRule.setContent {
            NixKeyTheme {
                SignRequestDialog(
                    queue = queue,
                    onApprove = { req ->
                        queue.complete(req.requestId, SignRequestStatus.APPROVED)
                    },
                    onDeny = {},
                )
            }
        }

        queue.enqueue(request1)
        queue.enqueue(request2)
        composeTestRule.waitForIdle()

        // First request shown
        composeTestRule.onNodeWithText("host-1").assertIsDisplayed()

        // Approve first
        composeTestRule.onNodeWithText("Approve").performClick()
        composeTestRule.waitForIdle()

        // Second request should now be shown
        composeTestRule.onNodeWithText("host-2").assertIsDisplayed()
    }

    @Test
    fun dialog_advancesAfterDeny() {
        val queue = SignRequestQueue()
        val request1 = makeRequest(hostName = "host-1")
        val request2 = makeRequest(hostName = "host-2")

        composeTestRule.setContent {
            NixKeyTheme {
                SignRequestDialog(
                    queue = queue,
                    onApprove = {},
                    onDeny = { req ->
                        queue.complete(req.requestId, SignRequestStatus.DENIED)
                    },
                )
            }
        }

        queue.enqueue(request1)
        queue.enqueue(request2)
        composeTestRule.waitForIdle()

        // First request shown
        composeTestRule.onNodeWithText("host-1").assertIsDisplayed()

        // Deny first
        composeTestRule.onNodeWithText("Deny").performClick()
        composeTestRule.waitForIdle()

        // Second request should now be shown
        composeTestRule.onNodeWithText("host-2").assertIsDisplayed()
    }

    @Test
    fun signRequest_dataHashIsTruncatedSha256() {
        val request = makeRequest(data = "hello world".toByteArray())
        val hash = request.dataHashTruncated()

        // SHA-256 of "hello world" starts with b94d27b9934d3e08
        assertTrue(hash.startsWith("b94d27b9934d3e08"))
        assertTrue(hash.endsWith("..."))
        // 16 hex chars + "..."
        assertEquals(19, hash.length)
    }
}
