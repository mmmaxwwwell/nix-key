package com.nixkey.keystore

import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.runner.AndroidJUnit4
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * T-AI-14: Display name editing test (FR-048).
 *
 * Verifies that a key's display name can be updated via [KeyManager.updateKey],
 * persisted, and reflected on re-read. Also tests that unlock and signing policies
 * can be independently updated.
 */
@RunWith(AndroidJUnit4::class)
class DisplayNameEditTest {

    private lateinit var keyManager: KeyManager
    private val createdAliases = mutableListOf<String>()

    @Before
    fun setUp() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        keyManager = KeyManager(context)
    }

    @After
    fun tearDown() {
        for (alias in createdAliases) {
            try {
                keyManager.deleteKey(alias)
            } catch (_: Exception) {
                // ignore cleanup errors
            }
        }
    }

    @Test
    fun updateDisplayName_persistsNewName() {
        val key = keyManager.createKey("original-name", KeyType.ECDSA_P256)
        createdAliases.add(key.alias)

        assertEquals("original-name", key.displayName)

        keyManager.updateKey(
            key.alias,
            displayName = "renamed-key",
            unlockPolicy = key.unlockPolicy,
            signingPolicy = key.confirmationPolicy,
        )

        val updated = keyManager.getKey(key.alias)
        assertNotNull("Key should still exist after rename", updated)
        assertEquals("renamed-key", updated!!.displayName)
    }

    @Test
    fun updateDisplayName_preservesOtherFields() {
        val key = keyManager.createKey(
            "test-preserve",
            KeyType.ED25519,
            unlockPolicy = UnlockPolicy.BIOMETRIC,
            signingPolicy = ConfirmationPolicy.PASSWORD,
        )
        createdAliases.add(key.alias)

        keyManager.updateKey(
            key.alias,
            displayName = "new-name",
            unlockPolicy = key.unlockPolicy,
            signingPolicy = key.confirmationPolicy,
        )

        val updated = keyManager.getKey(key.alias)!!
        assertEquals("new-name", updated.displayName)
        assertEquals(KeyType.ED25519, updated.keyType)
        assertEquals(key.fingerprint, updated.fingerprint)
        assertEquals(UnlockPolicy.BIOMETRIC, updated.unlockPolicy)
        assertEquals(ConfirmationPolicy.PASSWORD, updated.confirmationPolicy)
    }

    @Test
    fun updatePolicies_independentOfDisplayName() {
        val key = keyManager.createKey("policy-test", KeyType.ECDSA_P256)
        createdAliases.add(key.alias)

        // Change only policies, keep display name
        keyManager.updateKey(
            key.alias,
            displayName = key.displayName,
            unlockPolicy = UnlockPolicy.NONE,
            signingPolicy = ConfirmationPolicy.AUTO_APPROVE,
        )

        val updated = keyManager.getKey(key.alias)!!
        assertEquals("policy-test", updated.displayName)
        assertEquals(UnlockPolicy.NONE, updated.unlockPolicy)
        assertEquals(ConfirmationPolicy.AUTO_APPROVE, updated.confirmationPolicy)
    }

    @Test
    fun updateDisplayName_visibleInListKeys() {
        val key = keyManager.createKey("list-test", KeyType.ECDSA_P256)
        createdAliases.add(key.alias)

        keyManager.updateKey(
            key.alias,
            displayName = "updated-list",
            unlockPolicy = key.unlockPolicy,
            signingPolicy = key.confirmationPolicy,
        )

        val listed = keyManager.listKeys().find { it.alias == key.alias }
        assertNotNull("Key should appear in listKeys", listed)
        assertEquals("updated-list", listed!!.displayName)
    }

    @Test
    fun updateDisplayName_reflectedInExportPublicKey() {
        val key = keyManager.createKey("export-test", KeyType.ECDSA_P256)
        createdAliases.add(key.alias)

        keyManager.updateKey(
            key.alias,
            displayName = "new-comment",
            unlockPolicy = key.unlockPolicy,
            signingPolicy = key.confirmationPolicy,
        )

        val exported = keyManager.exportPublicKey(key.alias)
        // SSH public key format: "type base64 comment"
        val parts = exported.split(" ")
        assertEquals("Comment field should be updated display name", "new-comment", parts.last())
    }
}
