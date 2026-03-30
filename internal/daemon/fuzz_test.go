package daemon

import (
	"encoding/json"
	"testing"
)

// FuzzControlRequestParse tests that arbitrary JSON doesn't panic when parsed
// as a control socket Request.
func FuzzControlRequestParse(f *testing.F) {
	f.Add([]byte(`{"command":"list-devices"}`))
	f.Add([]byte(`{"command":"revoke-device","deviceId":"dev-123"}`))
	f.Add([]byte(`{"command":"get-status"}`))
	f.Add([]byte(`{"command":"get-keys"}`))
	f.Add([]byte(`{"command":"register-device","deviceId":"phone-1"}`))
	f.Add([]byte(`{"command":"get-device","deviceId":"dev-456"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"command":12345}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var req Request
		_ = json.Unmarshal(data, &req)
	})
}

// FuzzControlResponseParse tests that arbitrary JSON doesn't panic when
// parsed as a control socket Response.
func FuzzControlResponseParse(f *testing.F) {
	f.Add([]byte(`{"status":"ok"}`))
	f.Add([]byte(`{"status":"error","error":"not found"}`))
	f.Add([]byte(`{"status":"ok","data":{"running":true,"deviceCount":1}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		var resp Response
		_ = json.Unmarshal(data, &resp)
	})
}
