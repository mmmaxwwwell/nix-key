package mtls

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateIdentity(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "identity.txt")

	if err := GenerateIdentity(identityPath); err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	// File should exist with 0600 permissions
	info, err := os.Stat(identityPath)
	if err != nil {
		t.Fatalf("identity file not found: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected permissions 0600, got %o", perm)
	}

	// File should contain age identity data
	data, err := os.ReadFile(identityPath)
	if err != nil {
		t.Fatalf("failed to read identity file: %v", err)
	}
	if len(data) == 0 {
		t.Error("identity file is empty")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "identity.txt")

	if err := GenerateIdentity(identityPath); err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	// Write a test file
	original := []byte("secret private key data for testing")
	plainPath := filepath.Join(dir, "secret.pem")
	if err := os.WriteFile(plainPath, original, 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Encrypt the file
	if err := EncryptFile(plainPath, identityPath); err != nil {
		t.Fatalf("EncryptFile failed: %v", err)
	}

	// The encrypted file should exist at path + ".age"
	encryptedPath := plainPath + ".age"
	encData, err := os.ReadFile(encryptedPath)
	if err != nil {
		t.Fatalf("encrypted file not found: %v", err)
	}
	if len(encData) == 0 {
		t.Error("encrypted file is empty")
	}

	// Encrypted data should differ from original
	if string(encData) == string(original) {
		t.Error("encrypted data matches original (not encrypted)")
	}

	// Decrypt to memory
	decrypted, err := DecryptToMemory(encryptedPath, identityPath)
	if err != nil {
		t.Fatalf("DecryptToMemory failed: %v", err)
	}

	if string(decrypted) != string(original) {
		t.Errorf("decrypted data does not match original:\n  got:  %q\n  want: %q", decrypted, original)
	}
}

func TestDecryptWithWrongIdentity(t *testing.T) {
	dir := t.TempDir()

	// Create two different identities
	identity1 := filepath.Join(dir, "identity1.txt")
	identity2 := filepath.Join(dir, "identity2.txt")

	if err := GenerateIdentity(identity1); err != nil {
		t.Fatalf("GenerateIdentity 1 failed: %v", err)
	}
	if err := GenerateIdentity(identity2); err != nil {
		t.Fatalf("GenerateIdentity 2 failed: %v", err)
	}

	// Encrypt with identity1
	original := []byte("encrypted with identity1")
	plainPath := filepath.Join(dir, "secret.pem")
	if err := os.WriteFile(plainPath, original, 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if err := EncryptFile(plainPath, identity1); err != nil {
		t.Fatalf("EncryptFile failed: %v", err)
	}

	// Attempt to decrypt with identity2 should fail
	encryptedPath := plainPath + ".age"
	_, err := DecryptToMemory(encryptedPath, identity2)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong identity")
	}
}

func TestEncryptFileNotFound(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "identity.txt")

	if err := GenerateIdentity(identityPath); err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	err := EncryptFile(filepath.Join(dir, "nonexistent.pem"), identityPath)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestDecryptFileNotFound(t *testing.T) {
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "identity.txt")

	if err := GenerateIdentity(identityPath); err != nil {
		t.Fatalf("GenerateIdentity failed: %v", err)
	}

	_, err := DecryptToMemory(filepath.Join(dir, "nonexistent.age"), identityPath)
	if err == nil {
		t.Fatal("expected error for nonexistent encrypted file")
	}
}

func TestGenerateIdentity_InvalidPath(t *testing.T) {
	// Use /dev/null as parent directory — cannot create subdirectories inside a device file
	err := GenerateIdentity("/dev/null/identity.txt")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}
