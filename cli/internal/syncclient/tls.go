package syncclient

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
)

// BuildTLSConfig returns the TLS config used to connect to a self-signed
// rex-svr. Exactly one of the two trust modes must be requested:
//
//   - caCertPath: pins the exact leaf certificate at that path (the file the
//     server printed/shared via --ca-cert). Hostname/chain verification is
//     bypassed via InsecureSkipVerify because it's unnecessary once the
//     exact certificate bytes are pinned -- an attacker without that
//     certificate's private key cannot complete the handshake regardless of
//     what hostname they present.
//   - insecure: skips verification entirely, for convenience on a trusted
//     LAN when the operator hasn't distributed the cert file.
func BuildTLSConfig(caCertPath string, insecure bool) (*tls.Config, error) {
	if insecure {
		return &tls.Config{InsecureSkipVerify: true}, nil
	}
	if caCertPath == "" {
		return nil, fmt.Errorf("--ca-cert or --insecure is required to connect to a self-signed rex-svr (see the server's startup output for its certificate path)")
	}
	pemBytes, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read --ca-cert: %w", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("--ca-cert %s: not a PEM file", caCertPath)
	}
	pinned, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("--ca-cert %s: %w", caCertPath, err)
	}
	return &tls.Config{
		InsecureSkipVerify: true, // replaced by the exact-certificate pin below
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 || !bytes.Equal(rawCerts[0], pinned.Raw) {
				return fmt.Errorf("server certificate does not match --ca-cert %s", caCertPath)
			}
			return nil
		},
	}, nil
}
