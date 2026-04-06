package pairing

import (
	"encoding/json"
	"testing"
)

// FuzzQRPayloadParse tests that arbitrary JSON doesn't panic when parsed
// as a QR pairing payload.
func FuzzQRPayloadParse(f *testing.F) {
	f.Add([]byte(`{"v":1,"host":"100.64.0.1","port":29418,"cert":"AAAA","token":"tok123"}`))
	f.Add([]byte(`{"v":1,"host":"10.0.0.1","port":8080,"cert":"BBBB","token":"xyz","otel":"127.0.0.1:4317"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(``))
	f.Add([]byte(`{"v":999,"host":"","port":-1}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var p qrPayload
		_ = json.Unmarshal(data, &p)
	})
}
