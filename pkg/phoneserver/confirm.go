package phoneserver

// Confirmer is implemented by the Android side (BiometricHelper.kt + SignRequestDialog.kt)
// to prompt the user for sign request approval. gomobile will generate Java bindings.
type Confirmer interface {
	// RequestConfirmation shows a confirmation dialog for a sign request.
	// Returns true if the user approved, false if denied.
	// hostName is the display name of the requesting host.
	// keyName is the display name of the key being used.
	// dataHash is a truncated SHA-256 hex string of the data to sign.
	RequestConfirmation(hostName, keyName, dataHash string) (bool, error)
}
