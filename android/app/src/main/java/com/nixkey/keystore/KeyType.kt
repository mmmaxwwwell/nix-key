package com.nixkey.keystore

enum class KeyType(val sshName: String) {
    ED25519("ssh-ed25519"),
    ECDSA_P256("ecdsa-sha2-nistp256")
}
