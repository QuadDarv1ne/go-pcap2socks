// Package tlsutil provides TLS/HTTPS utility functions
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"
)

// GenerateSelfSignedCert generates a self-signed certificate and private key
// It returns the PEM-encoded certificate and key
func GenerateSelfSignedCert(host string) (certPEM, keyPEM []byte, err error) {
	// Generate private key
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate private key: %w", err)
	}

	// Certificate template
	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // Valid for 1 year

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"pcap2socks"},
			CommonName:   "localhost",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add IP addresses and DNS names
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = append(template.IPAddresses, ip)
	} else {
		template.DNSNames = append(template.DNSNames, host)
		// Always include localhost for convenience
		template.IPAddresses = append(template.IPAddresses, net.IPv4(127, 0, 0, 1))
		template.IPAddresses = append(template.IPAddresses, net.IPv6loopback)
	}

	// Create certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	// Encode certificate to PEM
	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})

	// Encode private key to PEM
	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal private key: %w", err)
	}

	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privBytes,
	})

	return certPEM, keyPEM, nil
}

// GenerateSelfSignedCertToFile generates a self-signed certificate and saves it to files
func GenerateSelfSignedCertToFile(certFile, keyFile string, hosts ...string) error {
	if len(hosts) == 0 {
		hosts = []string{"localhost"}
	}

	certPEM, keyPEM, err := GenerateSelfSignedCert(hosts[0])
	if err != nil {
		return err
	}

	// Write certificate file
	if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
		return fmt.Errorf("write certificate file: %w", err)
	}

	// Write key file
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("write key file: %w", err)
	}

	return nil
}

// CertExists checks if both certificate and key files exist
func CertExists(certFile, keyFile string) bool {
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		return false
	}
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return false
	}
	return true
}
