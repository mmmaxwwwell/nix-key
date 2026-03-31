package nixkeyv1

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

// FuzzProtoSignRequest tests that arbitrary bytes don't panic when
// unmarshaled as a SignRequest protobuf message.
func FuzzProtoSignRequest(f *testing.F) {
	// Seed: valid SignRequest (field 1=string "abc", field 2=bytes "hello", field 3=varint 0)
	f.Add([]byte{0x0a, 0x03, 0x61, 0x62, 0x63, 0x12, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x18, 0x00})
	// Seed: empty
	f.Add([]byte{})
	// Seed: single zero byte
	f.Add([]byte{0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 10*1024 {
			return // skip pathologically large inputs
		}
		var msg SignRequest
		_ = proto.Unmarshal(data, &msg)
	})
}

// FuzzProtoListKeysResponse tests that arbitrary bytes don't panic when
// unmarshaled as a ListKeysResponse protobuf message.
func FuzzProtoListKeysResponse(f *testing.F) {
	// Seed: valid ListKeysResponse with one SSHKey
	f.Add([]byte{0x0a, 0x06, 0x0a, 0x02, 0x41, 0x42, 0x12, 0x00})
	// Seed: empty
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 10*1024 {
			return // skip pathologically large inputs
		}
		var msg ListKeysResponse
		_ = proto.Unmarshal(data, &msg)
	})
}

// FuzzProtoRoundTrip verifies the property: decode(encode(x)) == x for protobuf messages.
// Starts from valid SignRequest data, unmarshals, re-marshals, and verifies equality.
func FuzzProtoRoundTrip(f *testing.F) {
	// Seed: valid SignRequest
	f.Add([]byte{0x0a, 0x03, 0x61, 0x62, 0x63, 0x12, 0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x18, 0x00})
	// Seed: empty (valid empty message)
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1024 {
			return // tighter limit for round-trip: proto.Equal is expensive on nested messages
		}
		var msg SignRequest
		if err := proto.Unmarshal(data, &msg); err != nil {
			return // invalid protobuf, skip
		}

		// Re-marshal
		encoded, err := proto.Marshal(&msg)
		if err != nil {
			t.Fatalf("Marshal failed after successful Unmarshal: %v", err)
		}

		// Unmarshal again
		var msg2 SignRequest
		if err := proto.Unmarshal(encoded, &msg2); err != nil {
			t.Fatalf("Unmarshal of re-marshaled data failed: %v", err)
		}

		// Verify equality
		if !proto.Equal(&msg, &msg2) {
			t.Fatalf("round-trip mismatch: original %v != re-decoded %v", &msg, &msg2)
		}
	})
}
