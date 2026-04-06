package mtls

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"filippo.io/age"
)

// GenerateIdentity creates a new age X25519 identity and writes it to path
// with 0600 permissions. The file contains the private identity in the standard
// age format (AGE-SECRET-KEY-1...).
func GenerateIdentity(path string) error {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return fmt.Errorf("generating age identity: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating identity directory: %w", err)
	}

	content := fmt.Sprintf("# created by nix-key\n# public key: %s\n%s\n", identity.Recipient(), identity)

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("writing age identity: %w", err)
	}

	return nil
}

// EncryptFile encrypts the file at path using the age identity's public key
// (recipient) and writes the encrypted output to path + ".age".
// The original file is not modified.
func EncryptFile(path, identityPath string) error {
	// Read the identity to extract the recipient (public key)
	identities, err := parseIdentities(identityPath)
	if err != nil {
		return fmt.Errorf("reading identity for encryption: %w", err)
	}

	// Extract recipient from the X25519 identity
	var recipients []age.Recipient
	for _, id := range identities {
		if x, ok := id.(*age.X25519Identity); ok {
			recipients = append(recipients, x.Recipient())
		}
	}
	if len(recipients) == 0 {
		return fmt.Errorf("no X25519 recipients found in identity file")
	}

	// Read the plaintext file
	plaintext, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading file to encrypt: %w", err)
	}

	// Encrypt
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipients...)
	if err != nil {
		return fmt.Errorf("creating age encryptor: %w", err)
	}
	if _, err := w.Write(plaintext); err != nil {
		return fmt.Errorf("writing encrypted data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing age encryptor: %w", err)
	}

	// Write encrypted file
	encPath := path + ".age"
	if err := os.WriteFile(encPath, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("writing encrypted file: %w", err)
	}

	return nil
}

// DecryptToMemory decrypts an age-encrypted file and returns the plaintext
// contents in memory. The decrypted data is never written to disk.
func DecryptToMemory(path, identityPath string) ([]byte, error) {
	identities, err := parseIdentities(identityPath)
	if err != nil {
		return nil, fmt.Errorf("reading identity for decryption: %w", err)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening encrypted file: %w", err)
	}
	defer f.Close()

	r, err := age.Decrypt(f, identities...)
	if err != nil {
		return nil, fmt.Errorf("decrypting file: %w", err)
	}

	plaintext, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading decrypted data: %w", err)
	}

	return plaintext, nil
}

// parseIdentities reads age identities from a file in the standard age format.
func parseIdentities(path string) ([]age.Identity, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening identity file: %w", err)
	}
	defer f.Close()

	identities, err := age.ParseIdentities(f)
	if err != nil {
		return nil, fmt.Errorf("parsing age identities: %w", err)
	}

	return identities, nil
}
