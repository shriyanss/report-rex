package syncserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
)

// EnsureSelfSignedCert loads certPath/keyPath if both already exist,
// otherwise generates a new ECDSA P-256 self-signed certificate and writes
// both files (the key at 0600). Clients pin the exact certificate bytes via
// --ca-cert rather than validating a hostname/chain, so no SAN handling is
// needed here -- a bare CN is enough.
//
// Returns the certificate's SHA-256 fingerprint (hex, colon-separated) for
// the operator to print and share with clients out-of-band.
func EnsureSelfSignedCert(certPath, keyPath string) (fingerprint string, err error) {
	if fileExists(certPath) && fileExists(keyPath) {
		der, err := certDERFromFile(certPath)
		if err != nil {
			return "", fmt.Errorf("read existing --cert %s: %w", certPath, err)
		}
		return fingerprintOf(der), nil
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		return "", fmt.Errorf("generate serial: %w", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "rex-svr"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return "", fmt.Errorf("create certificate: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", fmt.Errorf("marshal key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return "", fmt.Errorf("write --cert %s: %w", certPath, err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return "", fmt.Errorf("write --key %s: %w", keyPath, err)
	}

	return fingerprintOf(der), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func certDERFromFile(path string) ([]byte, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("not a PEM file")
	}
	return block.Bytes, nil
}

func fingerprintOf(der []byte) string {
	sum := sha256.Sum256(der)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	return strings.Join(parts, ":")
}
