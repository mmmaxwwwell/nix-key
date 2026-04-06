package pairing

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestGenerateQRPayload(t *testing.T) {
	params := QRParams{
		Host:  "100.64.0.1",
		Port:  8443,
		Cert:  "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----",
		Token: "abc123-one-time-token",
	}

	payload, err := GenerateQRPayload(params)
	if err != nil {
		t.Fatalf("GenerateQRPayload failed: %v", err)
	}

	// Payload should be valid base64
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("payload is not valid base64: %v", err)
	}

	// Decoded payload should be valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal(decoded, &result); err != nil {
		t.Fatalf("decoded payload is not valid JSON: %v", err)
	}

	// Verify all required fields
	if v, ok := result["v"]; !ok {
		t.Error("missing 'v' field")
	} else if v != float64(1) {
		t.Errorf("expected v=1, got %v", v)
	}

	if host, ok := result["host"]; !ok {
		t.Error("missing 'host' field")
	} else if host != "100.64.0.1" {
		t.Errorf("expected host=100.64.0.1, got %v", host)
	}

	if port, ok := result["port"]; !ok {
		t.Error("missing 'port' field")
	} else if port != float64(8443) {
		t.Errorf("expected port=8443, got %v", port)
	}

	if cert, ok := result["cert"]; !ok {
		t.Error("missing 'cert' field")
	} else if cert != params.Cert {
		t.Errorf("cert mismatch: got %v", cert)
	}

	if token, ok := result["token"]; !ok {
		t.Error("missing 'token' field")
	} else if token != "abc123-one-time-token" {
		t.Errorf("expected token=abc123-one-time-token, got %v", token)
	}

	// otel should not be present when not set
	if _, ok := result["otel"]; ok {
		t.Error("'otel' should not be present when OTELEndpoint is empty")
	}
}

func TestGenerateQRPayloadWithOTEL(t *testing.T) {
	params := QRParams{
		Host:         "100.64.0.2",
		Port:         9443,
		Cert:         "test-cert",
		Token:        "token-xyz",
		OTELEndpoint: "100.64.0.10:4317",
	}

	payload, err := GenerateQRPayload(params)
	if err != nil {
		t.Fatalf("GenerateQRPayload failed: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("payload is not valid base64: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(decoded, &result); err != nil {
		t.Fatalf("decoded payload is not valid JSON: %v", err)
	}

	if otel, ok := result["otel"]; !ok {
		t.Error("missing 'otel' field when OTELEndpoint is set")
	} else if otel != "100.64.0.10:4317" {
		t.Errorf("expected otel=100.64.0.10:4317, got %v", otel)
	}
}

func TestRenderQR(t *testing.T) {
	params := QRParams{
		Host:  "100.64.0.1",
		Port:  8443,
		Cert:  "test-cert",
		Token: "test-token",
	}

	output, err := RenderQR(params)
	if err != nil {
		t.Fatalf("RenderQR failed: %v", err)
	}

	if len(output) == 0 {
		t.Error("RenderQR returned empty output")
	}

	// QR terminal output should contain block characters
	// (the go-qrcode library uses Unicode block elements)
	if len(output) < 50 {
		t.Errorf("RenderQR output suspiciously short (%d bytes)", len(output))
	}
}

func TestGenerateQRPayloadEmptyFields(t *testing.T) {
	params := QRParams{
		Host:  "",
		Port:  0,
		Cert:  "",
		Token: "",
	}

	_, err := GenerateQRPayload(params)
	if err == nil {
		t.Error("expected error for empty required fields")
	}
}
