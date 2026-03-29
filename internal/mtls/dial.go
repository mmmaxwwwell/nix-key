package mtls

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// DialMTLS establishes a gRPC client connection using mTLS with cert pinning.
// If ageIdentityPath is non-empty, the client key is age-decrypted into memory
// before building the TLS config. The clientKeyPath should end in ".age" when
// age encryption is used. Additional gRPC dial options (e.g. otelgrpc interceptors)
// can be passed via extraOpts.
func DialMTLS(addr, clientCertPath, clientKeyPath, peerFingerprint, ageIdentityPath string, extraOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	certPEM, keyPEM, err := loadCertAndKey(clientCertPath, clientKeyPath, ageIdentityPath)
	if err != nil {
		return nil, fmt.Errorf("loading client cert/key: %w", err)
	}

	tlsCfg, err := PinnedTLSConfig(PinnedTLSOptions{
		CertPEM:         certPEM,
		KeyPEM:          keyPEM,
		PeerFingerprint: peerFingerprint,
		IsServer:        false,
	})
	if err != nil {
		return nil, fmt.Errorf("building client TLS config: %w", err)
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
	}
	opts = append(opts, extraOpts...)

	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("dialing gRPC: %w", err)
	}

	return conn, nil
}

// ListenMTLS creates a TLS listener for a gRPC server using mTLS with cert pinning.
// If ageIdentityPath is non-empty, the server key is age-decrypted into memory
// before building the TLS config.
func ListenMTLS(addr, serverCertPath, serverKeyPath, peerFingerprint, ageIdentityPath string) (net.Listener, error) {
	certPEM, keyPEM, err := loadCertAndKey(serverCertPath, serverKeyPath, ageIdentityPath)
	if err != nil {
		return nil, fmt.Errorf("loading server cert/key: %w", err)
	}

	tlsCfg, err := PinnedTLSConfig(PinnedTLSOptions{
		CertPEM:         certPEM,
		KeyPEM:          keyPEM,
		PeerFingerprint: peerFingerprint,
		IsServer:        true,
	})
	if err != nil {
		return nil, fmt.Errorf("building server TLS config: %w", err)
	}

	lis, err := tls.Listen("tcp", addr, tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("listening with TLS: %w", err)
	}

	return lis, nil
}

// loadCertAndKey reads the certificate and private key from disk.
// If ageIdentityPath is non-empty and keyPath ends in ".age", the key is
// age-decrypted into memory. Otherwise, the key file is read directly.
func loadCertAndKey(certPath, keyPath, ageIdentityPath string) (certPEM, keyPEM []byte, err error) {
	certPEM, err = readFile(certPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading cert: %w", err)
	}

	if ageIdentityPath != "" && strings.HasSuffix(keyPath, ".age") {
		keyPEM, err = DecryptToMemory(keyPath, ageIdentityPath)
		if err != nil {
			return nil, nil, fmt.Errorf("decrypting key with age: %w", err)
		}
	} else {
		keyPEM, err = readFile(keyPath)
		if err != nil {
			return nil, nil, fmt.Errorf("reading key: %w", err)
		}
	}

	return certPEM, keyPEM, nil
}

// readFile reads the entire file at path.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
