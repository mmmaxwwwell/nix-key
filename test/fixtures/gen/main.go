// Command gen generates deterministic test fixtures from fixed seeds.
//
// All cryptographic material is derived from a single master seed via
// ChaCha20-based CSPRNG so that re-running the generator produces
// byte-identical output every time. The only exception is the age identity
// (generated via CLI, not seeded); the full fixture set should be generated
// together and committed.
//
// Note: X.509 certs use Ed25519 (deterministic signing) to avoid
// non-determinism from Go's internal ECDSA randutil.MaybeReadByte.
//
// Usage:
//
//	go run ./test/fixtures/gen [output-dir]
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

const masterSeed = "nix-key-test-fixtures-deterministic-seed-v1"

// deterministicReader returns an io.Reader that produces deterministic bytes
// derived from the master seed + a domain-specific label.
func deterministicReader(label string) io.Reader {
	h := sha256.Sum256([]byte(masterSeed + ":" + label))
	nonce := make([]byte, chacha20.NonceSize)
	cipher, err := chacha20.NewUnauthenticatedCipher(h[:], nonce)
	if err != nil {
		panic(err)
	}
	return &chacha20Reader{cipher: cipher}
}

type chacha20Reader struct {
	cipher *chacha20.Cipher
}

func (r *chacha20Reader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	r.cipher.XORKeyStream(p, p)
	return len(p), nil
}

func main() {
	outDir := "test/fixtures"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}

	must(os.MkdirAll(outDir, 0o755))

	// Override crypto/rand.Reader with deterministic source for any library
	// code that reads from the global reader (e.g., ssh.MarshalPrivateKey
	// check bytes).
	origRand := rand.Reader
	defer func() { rand.Reader = origRand }()
	rand.Reader = deterministicReader("crypto-rand-global")

	// 1. Self-signed CA (Ed25519 for deterministic signing)
	fmt.Println("Generating CA cert...")
	caRng := deterministicReader("ca")
	caCert, caPriv := generateCA(caRng)
	writePEM(filepath.Join(outDir, "ca-cert.pem"), "CERTIFICATE", caCert.Raw)
	caKeyBytes, err := x509.MarshalPKCS8PrivateKey(caPriv)
	must(err)
	writePEM(filepath.Join(outDir, "ca-key.pem"), "PRIVATE KEY", caKeyBytes)

	// 2. Host client mTLS cert pair (Ed25519, signed by CA)
	fmt.Println("Generating host client cert...")
	hostCert, hostPriv := generateLeafCert(
		deterministicReader("host-client"),
		caCert, caPriv, "nix-key-host-client",
		x509.ExtKeyUsageClientAuth,
	)
	writePEM(filepath.Join(outDir, "host-client-cert.pem"), "CERTIFICATE", hostCert.Raw)
	hostKeyBytes, err := x509.MarshalPKCS8PrivateKey(hostPriv)
	must(err)
	writePEM(filepath.Join(outDir, "host-client-key.pem"), "PRIVATE KEY", hostKeyBytes)

	// 3. Phone server mTLS cert pair (Ed25519, signed by CA)
	fmt.Println("Generating phone server cert...")
	phoneCert, phonePriv := generateLeafCert(
		deterministicReader("phone-server"),
		caCert, caPriv, "nix-key-phone-server",
		x509.ExtKeyUsageServerAuth,
	)
	writePEM(filepath.Join(outDir, "phone-server-cert.pem"), "CERTIFICATE", phoneCert.Raw)
	phoneKeyBytes, err := x509.MarshalPKCS8PrivateKey(phonePriv)
	must(err)
	writePEM(filepath.Join(outDir, "phone-server-key.pem"), "PRIVATE KEY", phoneKeyBytes)

	// 4. SSH Ed25519 keypair
	fmt.Println("Generating SSH Ed25519 keypair...")
	ed25519Pub, ed25519Priv := generateEd25519SSH(deterministicReader("ssh-ed25519"))
	writeFile(filepath.Join(outDir, "ssh-ed25519"), marshalSSHPrivateKey(ed25519Priv, "test-ed25519"))
	writeFile(filepath.Join(outDir, "ssh-ed25519.pub"), ssh.MarshalAuthorizedKey(ed25519Pub))

	// 5. SSH ECDSA keypair (manually constructed to avoid MaybeReadByte)
	fmt.Println("Generating SSH ECDSA keypair...")
	ecdsaPub, ecdsaPriv := generateECDSASSHDeterministic(deterministicReader("ssh-ecdsa"))
	writeFile(filepath.Join(outDir, "ssh-ecdsa"), marshalSSHPrivateKey(ecdsaPriv, "test-ecdsa"))
	writeFile(filepath.Join(outDir, "ssh-ecdsa.pub"), ssh.MarshalAuthorizedKey(ecdsaPub))

	// Restore real randomness for age CLI
	rand.Reader = origRand

	// 6. Age identity + encrypted file
	fmt.Println("Generating age identity and encrypted file...")
	generateAgeFixtures(outDir)

	// 7. Adversarial cert fixtures
	fmt.Println("\nGenerating adversarial cert fixtures...")
	advDir := filepath.Join(outDir, "adversarial")
	must(os.MkdirAll(advDir, 0o755))
	generateAdversarialFixtures(advDir, caCert, caPriv)

	// 8. Print fixture summary
	printSummary(caCert, hostCert, phoneCert, ed25519Pub, ecdsaPub)

	fmt.Println("\nAll fixtures generated in", outDir)
}

func generateCA(rng io.Reader) (*x509.Certificate, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rng)
	must(err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"nix-key-test"},
			CommonName:   "nix-key Test CA",
		},
		NotBefore:             time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	// Ed25519 signing is deterministic — no randomness consumed from rng.
	certDER, err := x509.CreateCertificate(rng, template, template, pub, priv)
	must(err)

	cert, err := x509.ParseCertificate(certDER)
	must(err)

	return cert, priv
}

func generateLeafCert(rng io.Reader, caCert *x509.Certificate, caKey ed25519.PrivateKey, cn string, usage x509.ExtKeyUsage) (*x509.Certificate, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rng)
	must(err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"nix-key-test"},
			CommonName:   cn,
		},
		NotBefore:             time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{usage},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rng, template, caCert, pub, caKey)
	must(err)

	cert, err := x509.ParseCertificate(certDER)
	must(err)

	return cert, priv
}

func generateEd25519SSH(rng io.Reader) (ssh.PublicKey, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rng)
	must(err)

	sshPub, err := ssh.NewPublicKey(pub)
	must(err)

	return sshPub, priv
}

// generateECDSASSHDeterministic constructs an ECDSA-P256 key from raw bytes,
// bypassing ecdsa.GenerateKey which uses crypto/internal/randutil.MaybeReadByte
// (non-deterministic select on closed channel).
func generateECDSASSHDeterministic(rng io.Reader) (ssh.PublicKey, *ecdsa.PrivateKey) {
	curve := elliptic.P256()
	n := curve.Params().N

	// Read 32 bytes and derive a valid scalar d in [1, n-1].
	var seed [32]byte
	_, err := io.ReadFull(rng, seed[:])
	must(err)

	d := new(big.Int).SetBytes(seed[:])
	// Reduce to [1, n-1]: d = (d mod (n-1)) + 1
	nMinus1 := new(big.Int).Sub(n, big.NewInt(1))
	d.Mod(d, nMinus1)
	d.Add(d, big.NewInt(1))

	// Compute public key point.
	x, y := curve.ScalarBaseMult(d.Bytes())

	key := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: curve,
			X:     x,
			Y:     y,
		},
		D: d,
	}

	sshPub, err := ssh.NewPublicKey(&key.PublicKey)
	must(err)

	return sshPub, key
}

func marshalSSHPrivateKey(key interface{}, comment string) []byte {
	// ssh.MarshalPrivateKey reads from crypto/rand for check bytes.
	// We've overridden crypto/rand.Reader above for determinism.
	block, err := ssh.MarshalPrivateKey(key, comment)
	must(err)
	return pem.EncodeToMemory(block)
}

func generateAgeFixtures(outDir string) {
	identityPath := filepath.Join(outDir, "age-identity.txt")
	encryptedPath := filepath.Join(outDir, "age-encrypted.bin")
	plaintextPath := filepath.Join(outDir, "age-plaintext.txt")

	plaintext := []byte("nix-key test plaintext for age encryption fixture\n")
	writeFile(plaintextPath, plaintext)

	// age-keygen refuses to overwrite, so remove any existing file first.
	os.Remove(identityPath)

	cmd := exec.Command("age-keygen", "-o", identityPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "age-keygen failed: %s\n%s\n", err, output)
		os.Exit(1)
	}

	identityData, err := os.ReadFile(identityPath)
	must(err)
	recipient := extractAgeRecipient(string(identityData))

	cmd = exec.Command("age", "-r", recipient, "-o", encryptedPath)
	f, err := os.Open(plaintextPath)
	must(err)
	cmd.Stdin = f
	output, err = cmd.CombinedOutput()
	f.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "age encrypt failed: %s\n%s\n", err, output)
		os.Exit(1)
	}
}

func extractAgeRecipient(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "# public key: ") {
			return strings.TrimPrefix(line, "# public key: ")
		}
	}
	panic("could not extract age recipient from identity file")
}

func generateAdversarialFixtures(advDir string, caCert *x509.Certificate, caKey ed25519.PrivateKey) {
	// 1. Expired client cert — valid from 2020 to 2021 (already expired)
	fmt.Println("  [ADV-01] Expired client cert...")
	expiredCert, expiredKey := generateAdversarialLeafCert(
		deterministicReader("adv-expired"),
		caCert, caKey, "adv-expired-client",
		[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	writePEM(filepath.Join(advDir, "expired-client-cert.pem"), "CERTIFICATE", expiredCert.Raw)
	expiredKeyBytes, err := x509.MarshalPKCS8PrivateKey(expiredKey)
	must(err)
	writePEM(filepath.Join(advDir, "expired-client-key.pem"), "PRIVATE KEY", expiredKeyBytes)

	// 2. Not-yet-valid cert — valid from 2099 to 2100
	fmt.Println("  [ADV-02] Not-yet-valid cert...")
	futureCert, futureKey := generateAdversarialLeafCert(
		deterministicReader("adv-future"),
		caCert, caKey, "adv-future-client",
		[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	writePEM(filepath.Join(advDir, "future-client-cert.pem"), "CERTIFICATE", futureCert.Raw)
	futureKeyBytes, err := x509.MarshalPKCS8PrivateKey(futureKey)
	must(err)
	writePEM(filepath.Join(advDir, "future-client-key.pem"), "PRIVATE KEY", futureKeyBytes)

	// 3. Cert signed by a different (rogue) CA
	fmt.Println("  [ADV-03] Wrong-CA cert...")
	rogueCA, rogueCAKey := generateRogueCA(deterministicReader("adv-rogue-ca"))
	writePEM(filepath.Join(advDir, "rogue-ca-cert.pem"), "CERTIFICATE", rogueCA.Raw)
	rogueCAKeyBytes, err := x509.MarshalPKCS8PrivateKey(rogueCAKey)
	must(err)
	writePEM(filepath.Join(advDir, "rogue-ca-key.pem"), "PRIVATE KEY", rogueCAKeyBytes)

	wrongCACert, wrongCAKey := generateAdversarialLeafCert(
		deterministicReader("adv-wrong-ca"),
		rogueCA, rogueCAKey, "adv-wrong-ca-client",
		[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	writePEM(filepath.Join(advDir, "wrong-ca-client-cert.pem"), "CERTIFICATE", wrongCACert.Raw)
	wrongCAKeyBytes, err := x509.MarshalPKCS8PrivateKey(wrongCAKey)
	must(err)
	writePEM(filepath.Join(advDir, "wrong-ca-client-key.pem"), "PRIVATE KEY", wrongCAKeyBytes)

	// 4. Cert with wrong EKU (server auth instead of client auth)
	fmt.Println("  [ADV-04] Wrong-EKU cert...")
	wrongEKUCert, wrongEKUKey := generateAdversarialLeafCert(
		deterministicReader("adv-wrong-eku"),
		caCert, caKey, "adv-wrong-eku-client",
		[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, // should be ClientAuth
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	writePEM(filepath.Join(advDir, "wrong-eku-client-cert.pem"), "CERTIFICATE", wrongEKUCert.Raw)
	wrongEKUKeyBytes, err := x509.MarshalPKCS8PrivateKey(wrongEKUKey)
	must(err)
	writePEM(filepath.Join(advDir, "wrong-eku-client-key.pem"), "PRIVATE KEY", wrongEKUKeyBytes)

	// 5. Valid cert not in trust store (unpaired device) — signed by legitimate CA
	//    but represents a device that was never registered/paired
	fmt.Println("  [ADV-05] Unpaired device cert...")
	unpairedCert, unpairedKey := generateAdversarialLeafCert(
		deterministicReader("adv-unpaired"),
		caCert, caKey, "adv-unpaired-device",
		[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	writePEM(filepath.Join(advDir, "unpaired-client-cert.pem"), "CERTIFICATE", unpairedCert.Raw)
	unpairedKeyBytes, err := x509.MarshalPKCS8PrivateKey(unpairedKey)
	must(err)
	writePEM(filepath.Join(advDir, "unpaired-client-key.pem"), "PRIVATE KEY", unpairedKeyBytes)

	fmt.Printf("  All adversarial fixtures generated in %s\n", advDir)
}

func generateRogueCA(rng io.Reader) (*x509.Certificate, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rng)
	must(err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(99),
		Subject: pkix.Name{
			Organization: []string{"rogue-org"},
			CommonName:   "Rogue CA",
		},
		NotBefore:             time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rng, template, template, pub, priv)
	must(err)

	cert, err := x509.ParseCertificate(certDER)
	must(err)

	return cert, priv
}

// generateAdversarialLeafCert creates a leaf certificate with custom validity and EKU.
func generateAdversarialLeafCert(rng io.Reader, caCert *x509.Certificate, caKey ed25519.PrivateKey, cn string, ekus []x509.ExtKeyUsage, notBefore, notAfter time.Time) (*x509.Certificate, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rng)
	must(err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(100), // distinct from normal fixtures
		Subject: pkix.Name{
			Organization: []string{"nix-key-test"},
			CommonName:   cn,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           ekus,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rng, template, caCert, pub, caKey)
	must(err)

	cert, err := x509.ParseCertificate(certDER)
	must(err)

	return cert, priv
}

func printSummary(caCert, hostCert, phoneCert *x509.Certificate, ed25519Pub, ecdsaPub ssh.PublicKey) {
	fmt.Println("\n--- Fixture Summary ---")
	fmt.Printf("CA Subject:      %s\n", caCert.Subject.CommonName)
	fmt.Printf("CA Fingerprint:  %x\n", sha256.Sum256(caCert.Raw))
	fmt.Printf("Host Client CN:  %s\n", hostCert.Subject.CommonName)
	fmt.Printf("Phone Server CN: %s\n", phoneCert.Subject.CommonName)
	fmt.Printf("SSH Ed25519:     %s\n", ssh.FingerprintSHA256(ed25519Pub))
	fmt.Printf("SSH ECDSA:       %s\n", ssh.FingerprintSHA256(ecdsaPub))
}

func writePEM(path, pemType string, data []byte) {
	block := &pem.Block{
		Type:  pemType,
		Bytes: data,
	}
	writeFile(path, pem.EncodeToMemory(block))
}

func writeFile(path string, data []byte) {
	must(os.WriteFile(path, data, 0o600))
}

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
