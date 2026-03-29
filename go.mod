module github.com/phaedrus-raznikov/nix-key

go 1.24.6

require (
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	golang.org/x/crypto v0.49.0
)

require golang.org/x/sys v0.42.0 // indirect

replace github.com/skip2/go-qrcode => /tmp/go-qrcode
