// Package phoneserver implements the NixKeyAgent gRPC service for the phone side.
// It is designed to be compiled via gomobile for use in Android.
package phoneserver

// KeyStore is implemented by the Android side (KeyManager.kt) to provide
// SSH key operations. gomobile will generate Java bindings for this interface.
type KeyStore interface {
	// ListKeys returns the number of available keys. Use KeyAt to get each key.
	ListKeys() (*KeyList, error)
	// Sign signs data with the key identified by fingerprint.
	// Returns the raw signature bytes.
	Sign(fingerprint string, data []byte, flags int32) ([]byte, error)
}

// KeyList is a gomobile-friendly list of SSH keys.
// gomobile cannot export Go slices of custom types, so we use an accessor pattern.
type KeyList struct {
	keys []*Key
}

// NewKeyList creates an empty KeyList.
func NewKeyList() *KeyList {
	return &KeyList{}
}

// Add appends a key to the list.
func (kl *KeyList) Add(k *Key) {
	kl.keys = append(kl.keys, k)
}

// Len returns the number of keys.
func (kl *KeyList) Len() int {
	if kl == nil {
		return 0
	}
	return len(kl.keys)
}

// Get returns the key at the given index.
func (kl *KeyList) Get(i int) *Key {
	if kl == nil || i < 0 || i >= len(kl.keys) {
		return nil
	}
	return kl.keys[i]
}

// Key represents an SSH public key on the phone.
type Key struct {
	PublicKeyBlob []byte
	KeyType       string
	DisplayName   string
	Fingerprint   string
}
