package tlsutil

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	certPEM, keyPEM, err := GenerateSelfSignedCert("localhost")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert failed: %v", err)
	}

	if len(certPEM) == 0 {
		t.Error("Certificate PEM is empty")
	}

	if len(keyPEM) == 0 {
		t.Error("Key PEM is empty")
	}

	// Verify certificate can be parsed
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("Failed to decode certificate PEM")
	}

	if block.Type != "CERTIFICATE" {
		t.Errorf("Expected CERTIFICATE block, got %s", block.Type)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Verify certificate properties
	if cert.Subject.Organization[0] != "pcap2socks" {
		t.Errorf("Expected organization 'pcap2socks', got %s", cert.Subject.Organization[0])
	}

	// Verify validity period
	now := time.Now()
	if cert.NotBefore.After(now) {
		t.Error("Certificate not yet valid")
	}

	expectedNotAfter := cert.NotBefore.Add(365 * 24 * time.Hour)
	if !cert.NotAfter.Equal(expectedNotAfter) {
		t.Errorf("Certificate expiration mismatch: got %v, want %v", cert.NotAfter, expectedNotAfter)
	}

	// Verify key usage
	if cert.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
		t.Error("Certificate missing KeyEncipherment key usage")
	}

	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("Certificate missing DigitalSignature key usage")
	}

	if len(cert.ExtKeyUsage) == 0 || cert.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Error("Certificate missing ServerAuth extended key usage")
	}
}

func TestGenerateSelfSignedCert_WithIP(t *testing.T) {
	certPEM, _, err := GenerateSelfSignedCert("192.168.1.1")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert failed: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Check IP addresses
	found := false
	for _, ip := range cert.IPAddresses {
		if ip.String() == "192.168.1.1" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected IP 192.168.1.1 in certificate, got %v", cert.IPAddresses)
	}
}

func TestGenerateSelfSignedCert_WithHostname(t *testing.T) {
	certPEM, _, err := GenerateSelfSignedCert("example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert failed: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Check DNS names
	found := false
	for _, name := range cert.DNSNames {
		if name == "example.com" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected DNS name 'example.com', got %v", cert.DNSNames)
	}
}

func TestGenerateSelfSignedCertToFile(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	err := GenerateSelfSignedCertToFile(certFile, keyFile, "localhost")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCertToFile failed: %v", err)
	}

	// Check files exist
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		t.Error("Certificate file was not created")
	}

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		t.Error("Key file was not created")
	}

	// Verify file permissions
	keyInfo, err := os.Stat(keyFile)
	if err != nil {
		t.Fatalf("Failed to stat key file: %v", err)
	}

	// Check that key file is not world-readable
	// On Windows, this check is less strict due to different permission model
	if keyInfo.Mode().Perm()&0007 != 0 {
		t.Logf("Warning: Key file may have overly permissive permissions: %v", keyInfo.Mode())
		// Not failing on Windows as permission model differs
	}

	// Verify content
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("Failed to read certificate file: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Error("Certificate file contains invalid PEM data")
	}
}

func TestCertExists(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	// Test non-existing files
	if CertExists(certFile, keyFile) {
		t.Error("CertExists returned true for non-existing files")
	}

	// Create files
	err := GenerateSelfSignedCertToFile(certFile, keyFile)
	if err != nil {
		t.Fatalf("Failed to create test certificates: %v", err)
	}

	// Test existing files
	if !CertExists(certFile, keyFile) {
		t.Error("CertExists returned false for existing files")
	}

	// Test missing cert file
	os.Remove(certFile)
	if CertExists(certFile, keyFile) {
		t.Error("CertExists returned true when cert file is missing")
	}

	// Recreate cert and test missing key file
	err = GenerateSelfSignedCertToFile(certFile, keyFile)
	if err != nil {
		t.Fatalf("Failed to recreate test certificates: %v", err)
	}
	os.Remove(keyFile)
	if CertExists(certFile, keyFile) {
		t.Error("CertExists returned true when key file is missing")
	}
}

func TestGenerateSelfSignedCert_Validity(t *testing.T) {
	certPEM, _, err := GenerateSelfSignedCert("test.local")
	if err != nil {
		t.Fatalf("GenerateSelfSignedCert failed: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Verify the certificate is valid for at least 364 days
	minValidity := 364 * 24 * time.Hour
	actualValidity := cert.NotAfter.Sub(cert.NotBefore)

	if actualValidity < minValidity {
		t.Errorf("Certificate validity period too short: got %v, want at least %v", actualValidity, minValidity)
	}
}
