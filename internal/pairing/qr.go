package pairing

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	qrcode "github.com/skip2/go-qrcode"
)

// QRParams holds the parameters for generating a pairing QR code.
type QRParams struct {
	Host         string
	Port         int
	Cert         string
	Token        string
	OTELEndpoint string
}

// qrPayload is the JSON structure encoded in the QR code.
type qrPayload struct {
	V     int    `json:"v"`
	Host  string `json:"host"`
	Port  int    `json:"port"`
	Cert  string `json:"cert"`
	Token string `json:"token"`
	OTEL  string `json:"otel,omitempty"`
}

// GenerateQRPayload encodes the pairing parameters as a Base64-encoded JSON string.
func GenerateQRPayload(params QRParams) (string, error) {
	if params.Host == "" || params.Port == 0 || params.Cert == "" || params.Token == "" {
		return "", fmt.Errorf("pairing payload: host, port, cert, and token are required")
	}

	p := qrPayload{
		V:     1,
		Host:  params.Host,
		Port:  params.Port,
		Cert:  params.Cert,
		Token: params.Token,
		OTEL:  params.OTELEndpoint,
	}

	jsonBytes, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("pairing payload: marshal JSON: %w", err)
	}

	return base64.StdEncoding.EncodeToString(jsonBytes), nil
}

// RenderQR generates a pairing QR code and returns its terminal-printable string representation.
func RenderQR(params QRParams) (string, error) {
	payload, err := GenerateQRPayload(params)
	if err != nil {
		return "", err
	}

	qr, err := qrcode.New(payload, qrcode.Medium)
	if err != nil {
		return "", fmt.Errorf("pairing QR: generate code: %w", err)
	}

	return qr.ToSmallString(false), nil
}
